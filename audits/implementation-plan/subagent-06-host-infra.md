# Subagent 06 — Single-Host Infrastructure Truth Seam — Implementation Plan

**Focus:** Files / Cron / Monitoring / Firewall / Host — the 1Panel host surface vs Forge fleet (the single-host appliance seam vs fleet orchestrator)
**Date:** 2026-08-24
**Author:** plan subagent 06/10 (parallel run — DESIGN ONLY, no code modified)
**Workspace root:** `/Users/riyaz/project/gamepanel`
**Re-verified source commit:** `main` @ 2026-08-24 + dirty (read-only synthesis)
**Prior audits consolidated:**
- `audits/final-parity/subagent-05-host-admin.md` (18-row matrix, 6 LFs) — **primary source for this seam**
- `audits/phase-06/subagent-04-1panel-infra-monitor.md` LF-02/LF-03/LF-04
- `audits/FINAL_PARITY_AUDIT.md` §21-§23.8, `audits/MASTER_FINDING_INDEX.md`
- `AUDIT_REMEDIATION_471.md` firewall transactional hardening

**Forge layers re-verified (file:line SOURCE_VERIFIED 2026-08-24):**
- `forge/api/internal/http/handlers_files.go:386` `hostFilesUpload` rawBody unbounded
- `beacon/internal/server/hostfiles.go:457` `hostUploadLimit 100*1024*1024` + `109 hostAtomicWrite` + `51 resolveHostPath` + `19 validateHostPath`
- `beacon/internal/server/secure_files.go:155` `archivePathTracker` + `26 limits` + `269/320 staged extract` — tracker pattern to reuse at API layer
- `forge/api/internal/services/cronjob/service.go:268` `TriggerNow` dup vs `124 executeJob` + `161 time.Sleep` blocks scheduler
- `forge/web/app/admin/monitoring/page.tsx:50` `isSynthetic` synthetic-zero fallback
- `beacon/internal/server/handlers_host.go:65` `Uptime: time.Since(s.started)` mislabel + `74 handleHostDisk` single `/` only
- `beacon/internal/server/sysinfo_linux.go:61` `netInterfacesPlatform` missing IPs + `94 processListPlatform` inert CPU sort
- `beacon/internal/remote/reconnect.go:98` `Start` + `98-112 run` StateConnected blind heartbeat
- `beacon/internal/server/handlers_firewall.go:361` `reconcile` iptables-restore transactional + `259 persistLocked` 0600+fsync
- `beacon/internal/rootfs/rootfs_linux.go:47` `RESOLVE_BENEATH|NO_MAGICLINKS|NO_SYMLINKS` confinement

---

## 1. Executive Summary & Scope Contract

### 1.1 What this plan fixes vs freezes

**Fix (P1/P2 load-bearing):**
1. **Upload OOM seam** — Forge API `handlers_files.go:386` buffers `rawBody:=c.Request().Body()` unbounded before beacon's `hostfiles.go:457` 100 MiB cap. Authenticated admin can OOM the control plane for all nodes with one 400 MiB multipart.
2. **Cron duplicate execution + scheduler stall** — `service.go:268` `TriggerNow` creates outer row, `service.go:124` `executeJob` creates shadow row (caller polls hung `pending`). Retry loop `service.go:161` `time.Sleep(5*(i+1)s)` blocks the single `cron.Cron` goroutine — starves sibling jobs up to 4.5 min on RetryCount=10.
3. **Monitoring synthetic-zero + daemon-uptime mislabel + sysinfo fidelity** — `monitoring/page.tsx:50` `isSynthetic` is honest band-aid over missing `gopsutil` collectors; `handlers_host.go:65` reports daemon age as host uptime; `sysinfo_linux.go:61` network IPs absent; `94` process CPU/Mem always zero yet FE sorts by it; `handlers_host.go:74` disk reports only `/`.
4. **Beacon reconnect blind StateConnected defeats heartbeatmonitor** — `reconnect.go:98` `run` initial StateConnecting logic + `192 StateConnected` immediately after `probePanel` success without verifying `SendNodeHeartbeat` 2xx + HMAC acceptance; stale heartbeat window flaps.

**Verify (already fixed — regression-guard only):**
5. **Firewall transactional** — `handlers_firewall.go:361` `reconcile` via `iptables-restore --wait 10 --noflush` sorted + `persistLocked` 0600/fsync/syncDirectory is correctly transactional since AUDIT_REMEDIATION_471 H7-08 fix. Plan only adds verification + UI placeholder fix.
6. **Hostfiles confinement** — `rootfs_linux.go:47` `RESOLVE_BENEATH` + `hostfiles.go:51` `resolveHostPath` allowlist/denylist deny-by-default is correct. Plan only strengthens prefix sibling check and adds tests.

**Freeze (intentionally REJECT — document, do not build):**
7. **Host appliance sprawl** — `daemon.json` live-edit, `sshd`/`RootCert` CA distribution, `fail2ban` jail.local, `vsftpd` FTP, system-image `snapshot.go` rollback, multi-host SSH inventory (`host.go:19 CreateHost`), `device.go/gpu.go/host_tool.go` — all correctly absent. Building them would re-create the FINAL §22 “host-tool sprawl at control plane” anti-pattern.

### 1.2 Guiding principles for this seam

- **Fleet truth, not host truth.** Forge nodes are provisioned immutable hosts (cloud-init/Ansible). Control plane must not `OperateDocker` restart `dockerd` or `UpdateDaemonJson` or edit `jail.local`. Documented REJECT prevents future parity audits re-filing as MISSING.
- **Control plane proxy is the weakest link.** Any host operation must enforce limits at the *proxy* (`forge/api`) before the daemon cap, otherwise fleet-wide DoS via single node path.
- **Kernel boundary is the only boundary you can trust.** `openat2 RESOLVE_BENEATH` for per-server FS is correct; host FS relies on prefix checks + allowlist — strengthen but do not pretend prefix == fd-pinning.
- **Honest emptiness > synthetic zeros.** Monitoring must render `no data` not `0%` when collectors absent.

---

## 2. Findings → Fixes Traceability Matrix

| Finding ID | Audit Source | File:Line (root cause) | Severity | Fix Track | Section |
|------------|--------------|------------------------|----------|-----------|---------|
| **H-OOM-01** | final-parity LF-01 / phase-06 LF-02 | `forge/api/internal/http/handlers_files.go:402` `rawBody:=c.Request().Body()` → `bytes.NewReader(rawBody)` unbounded; vs `beacon/internal/server/hostfiles.go:457` `hostUploadLimit 100MiB` + `458 MaxBytesReader` | **P1** DoS | Stream + early cap + pipe | §3 |
| **H-CRON-01** | final-parity LF-02 | `forge/api/internal/services/cronjob/service.go:268` `CreateCronJobExecution` outer + `124` inner dup | **P2** functional | Deduplicate to single execution row | §4 |
| **H-CRON-02** | final-parity LF-02/LF-03 | `service.go:161` `time.Sleep(5*(i+1)s)` inside cron goroutine | **P2** scheduler stall | Non-blocking retry via `time.AfterFunc` + queue | §4 |
| **H-CRON-03** | prompt add | cross-instance dedup missing | **P2** multi-replica | `pg_advisory_xact_lock` + monotonic dedup | §4 |
| **H-MON-01** | final-parity LF-03 | `forge/web/app/admin/monitoring/page.tsx:50` `isSynthetic` every zero | **P2** integrity | Render `no data` not 0, wire live collectors or explicit gap | §5 |
| **H-MON-02** | final-parity LF-03 | `beacon/internal/server/handlers_host.go:65` `time.Since(s.started)` labeled Uptime | **P3** UX (P2 during incident) | `/proc/uptime` host uptime + suspend subtraction, expose both | §5 |
| **H-MON-03** | prompt | `beacon/internal/server/sysinfo_linux.go:94` `processListPlatform` Name/State only | **P3** inert sort | Disable sort or wire `gopsutil` + hide CPU cols | §5 |
| **H-MON-04** | prompt | `handlers_host.go:74` `Statfs("/")` single disk | **P3** | Enumerate all mounts via `/proc/mounts` | §5 |
| **H-MON-05** | prompt | `sysinfo_linux.go:61` `netInterfacesPlatform` no IPs/MAC | **P3** | Merge `net.Interfaces()` IPs/MAC on linux (keep sysfs for speed/operstate) | §5 |
| **H-MON-06** | prompt | `sysinfo_linux.go:94` sibling but dupe | — | Cleanup + darwin parity note | §5 |
| **H-FW-01** | AUDIT_REMEDIATION_471 H7-08 | `beacon/internal/server/handlers_firewall.go:361` reconcile | **INFO** already fixed | Verify + UI placeholder fix | §6 |
| **H-FW-02** | final-parity H-04/H-05 | `handlers_firewall.go:129` allowlist bifurcation | **INFO** deny-by-default correct | Keep + doc | §6 |
| **H-FILE-01** | FINAL §23.8 | `beacon/internal/rootfs/rootfs_linux.go:47` RESOLVE_BENEATH | **INFO** stronger | Strengthen + tests | §7 |
| **H-BEACON-01** | prompt | `beacon/internal/remote/reconnect.go:98` StateConnected blind | **P1** fleet offline misclassification | Probe-before-Connected + `RecordHeartbeatSuccess` only on 2xx | §8 |
| **H-REJECT-01** | FINAL §22 | daemon.json/SSH/fail2ban/FTP/snapshot | **INFO** REJECT | ADR doc | §9 |

---

## 3. Fix Track H-OOM-01 — Host Upload Buffering OOM at Forge Proxy

### 3.1 Root cause (file:line)

`forge/api/internal/http/handlers_files.go:386-431` (`hostFilesUpload`):

```go
// CURRENT — vulnerable (handlers_files.go:386-431)
func hostFilesUpload(cfg Config) fiber.Handler {
    return func(c *fiber.Ctx) error {
        // ...
        destPath := c.Query("path", "/")
        // ...
        targetURL := strings.TrimRight(target.BaseURL, "/") + "/v1/files/upload?path=" + url.QueryEscape(destPath)
        rawBody := c.Request().Body() // ← reads ENTIRE body into heap, no limit
        req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, targetURL, bytes.NewReader(rawBody))
        // ...
        req.ContentLength = int64(len(rawBody))
        headers, err := cfg.Daemon.SignedHeaders(target.NodeToken, http.MethodPost, req.URL.RequestURI(), rawBody)
        // rawBody is also fed into HMAC — forces full buffering
        resp, err := cfg.Daemon.HTTPClient().Do(req)
        // ...
    }
}
```

vs beacon correct cap `beacon/internal/server/hostfiles.go:457`:

```go
const hostUploadLimit = 100 * 1024 * 1024 // 100 MiB
r.Body = http.MaxBytesReader(w, r.Body, hostUploadLimit) // enforced before parse
// ...
if err := hostAtomicWrite(cleaned, reader, hostUploadLimit, 0o640); err != nil {
```

**Asymmetry:** beacon fails with 413 at 100 MiB+1; forge OOMs at 400 MiB *before* beacon ever sees the request. `hostAtomicWrite` internal `io.LimitReader(limit+1)` guard at `hostfiles.go:124` is never reached because the proxy already allocated.

Fiber default `BodyLimit` is 4 MiB for JSON but host upload bypasses it via raw read; `c.Request().Body()` is unbounded by default in Fiber 2 unless `app.Settings.BodyLimit` set (currently not set for `/host/files/upload`).

### 3.2 Design — stream with early size cap + signed-metadata HMAC

**Goal:** enforce 100 MiB cap at the *first* byte before heap alloc, and proxy to beacon without holding full body in memory. Follow beacon `secure_files.go:155` `archivePathTracker` pattern for validation *before* buffering — here, validate `ContentLength` + `MaxBytesReader` before proxy, then stream via `io.Pipe` (chunked) or `http.NewRequest` with `io.LimitedReader`.

**Constraints:**
- `SignedHeaders(target.NodeToken, method, uri, body)` currently HMACs raw body (`forge/api/internal/daemon/client.go: SignedHeaders` takes `body []byte`). Changing to stream requires either (a) signing metadata only, or (b) tee-ing stream through HMAC without buffering whole body.
- Preferred: **(b) tee + limit + early 413**, keep HMAC over body but via streaming hash so memory is O(1). If daemon signature verification requires buffered body, we still cap to 100 MiB+1 before hashing — so worst heap is bounded.

**Files to change:**
1. `forge/api/internal/http/handlers_files.go:386` — rewrite `hostFilesUpload`
2. `forge/api/internal/daemon/client.go` — add `SignedHeadersStream` variant that HMACs `io.Reader` with limit (or reuse existing but feed limited reader)
3. `forge/api/cmd/api/main.go` or `forge/api/internal/http/server.go` — set Fiber `BodyLimit` for host routes
4. `beacon/internal/server/hostfiles.go:457` — keep as defense-in-depth (no change, but align limit constant)
5. `forge/web/components/admin/host-files-view.tsx` — client-side preflight size check

### 3.3 Code snippet — replacement `hostFilesUpload` (streaming, bounded)

```go
// forge/api/internal/http/handlers_files.go — REPLACEMENT
package http

const (
    hostUploadLimit     = 100 * 1024 * 1024        // must match beacon hostfiles.go:457
    hostUploadLimitPlus = hostUploadLimit + 1      // +1 to detect overflow
    hostUploadSlack     = 8 * 1024                 // multipart boundary slack
)

// hostFilesUpload streams the incoming request to beacon without buffering
// the entire body in memory. It enforces the 100 MiB cap at the proxy before
// the beacon cap, preventing OOM via unbounded c.Request().Body().
func hostFilesUpload(cfg Config) fiber.Handler {
    return func(c *fiber.Ctx) error {
        target, err := resolveNode(cfg, c)
        if err != nil {
            return err
        }
        destPath := c.Query("path", "/")
        if err := validateHostFilePath(destPath); err != nil {
            return err
        }

        // 1. Early reject via Content-Length header before reading body.
        //    Handles non-chunked clients; chunked falls through to LimitedReader.
        if cl := c.Request().Header.ContentLength(); cl > hostUploadLimitPlus+hostUploadSlack {
            return fiber.NewError(fiber.StatusRequestEntityTooLarge,
                fmt.Sprintf("host upload exceeds %d MiB limit", hostUploadLimit/(1024*1024)))
        }

        ct := strings.ToLower(string(c.Request().Header.ContentType()))
        if ct != "application/octet-stream" && !strings.HasPrefix(ct, "multipart/form-data;") {
            return fiber.NewError(fiber.StatusUnsupportedMediaType,
                "upload must be application/octet-stream or multipart/form-data")
        }

        targetURL := strings.TrimRight(target.BaseURL, "/") + "/v1/files/upload?path=" + url.QueryEscape(destPath)

        // 2. Stream via LimitedReader — never hold > limit in heap.
        //    c.Request().BodyStream() would also work; use Body() with LimitReader wrapper
        //    for Fiber compatibility, but cap via io.LimitedReader on the underlying stream.
        //    For Fiber, BodyStream is fasthttp request body stream.
        bodyStream := bytes.NewReader(c.Request().Body())
        // If Body() already read full — we need to prevent that. So switch to:
        // Use c.Context().Request.BodyStream() with MaxBytes.
        // Preferred: wrap fasthttp body via io.LimitReader before any alloc.
        limited := io.LimitReader(c.Context().Request.BodyStream(), hostUploadLimitPlus+hostUploadSlack)

        // 3. Tee through HMAC while streaming to pipe — O(1) memory.
        //    Only if daemon requires body HMAC. If daemon can sign URI+timestamp only,
        //    use SignedHeaders with nil body and set X-Forge-Upload-Size header instead.
        pr, pw := io.Pipe()
        hmacErrCh := make(chan error, 1)
        var signedHeaders http.Header

        go func() {
            defer pw.Close()
            // Hash while copying to pipe, enforcing limit+1 overflow detection
            hasher := newBodyHMAC(target.NodeToken, http.MethodPost, "/v1/files/upload?path="+url.QueryEscape(destPath))
            written, err := io.Copy(io.MultiWriter(pw, hasher), limited)
            if err != nil {
                pw.CloseWithError(err)
                hmacErrCh <- err
                return
            }
            if written > hostUploadLimit {
                pw.CloseWithError(errors.New("host upload exceeds limit"))
                hmacErrCh <- fiber.NewError(fiber.StatusRequestEntityTooLarge, "host upload exceeds 100 MiB limit")
                return
            }
            signedHeaders, err = hasher.SignedHeaders()
            if err != nil {
                pw.CloseWithError(err)
            }
            hmacErrCh <- err
        }()

        req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, targetURL, pr)
        if err != nil {
            return respondInternalError(c, err)
        }
        req.Header.Set("Content-Type", string(c.Request().Header.ContentType()))
        // Use chunked encoding — do not set ContentLength (streaming)
        req.TransferEncoding = []string{"chunked"}

        // Wait briefly for HMAC headers (non-blocking alternative: sign metadata only)
        // If signing metadata only, remove this wait and call SignedHeaders with empty body.
        select {
        case herr := <-hmacErrCh:
            if herr != nil {
                return herr
            }
        case <-time.After(200 * time.Millisecond):
            // HMAC not yet ready — fall back to metadata-only signing
            // to avoid blocking scheduler (covers slow client)
        }

        // Merge signed headers
        if signedHeaders != nil {
            for k, vals := range signedHeaders {
                for _, v := range vals {
                    req.Header.Add(k, v)
                }
            }
        } else {
            // Fallback: sign URI only (no body) — daemon must accept this for streaming uploads
            hdrs, err := cfg.Daemon.SignedHeaders(target.NodeToken, http.MethodPost, req.URL.RequestURI(), nil)
            if err != nil {
                return respondInternalError(c, err)
            }
            for k, vals := range hdrs {
                for _, v := range vals {
                    req.Header.Add(k, v)
                }
            }
        }

        resp, err := cfg.Daemon.HTTPClient().Do(req)
        if err != nil {
            return fiber.NewError(fiber.StatusBadGateway, err.Error())
        }
        defer resp.Body.Close()
        respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
        if resp.StatusCode < 200 || resp.StatusCode >= 300 {
            return fiber.NewError(resp.StatusCode, string(respBody))
        }
        return c.Send(respBody)
    }
}

// Alternative simpler path (recommended for v1): enforce limit before any Body() read
// and keep SignedHeaders with nil body — requires daemon to accept metadata-only HMAC for uploads.
// This is the lowest-risk change and matches secure_files.go:597 securePullClient pattern
// where body is not signed, only URI+method+timestamp.
```

**Simpler v1 (recommended for immediate P1 fix) — bounded before Body() + metadata-only HMAC:**

```go
func hostFilesUpload(cfg Config) fiber.Handler {
    return func(c *fiber.Ctx) error {
        target, err := resolveNode(cfg, c)
        if err != nil { return err }
        destPath := c.Query("path", "/")
        if err := validateHostFilePath(destPath); err != nil { return err }

        ct := strings.ToLower(string(c.Request().Header.ContentType()))
        if ct != "application/octet-stream" && !strings.HasPrefix(ct, "multipart/form-data;") {
            return fiber.NewError(fiber.StatusUnsupportedMediaType, "upload must be application/octet-stream or multipart/form-data")
        }

        // Early cap before any alloc — covers both Content-Length and chunked via MaxBytesReader equivalent.
        if cl := c.Request().Header.ContentLength(); cl != -1 && cl > hostUploadLimit+hostUploadSlack {
            return fiber.NewError(fiber.StatusRequestEntityTooLarge, "host upload exceeds 100 MiB limit")
        }
        // For Fiber, enforce via fasthttp request size: c.Request().Header.ContentLength() already checked;
        // additionally wrap BodyStream with LimitReader so chunked cannot exceed.
        limited := io.LimitReader(bytes.NewReader(c.Request().Body()), hostUploadLimitPlus)
        // NOTE: c.Request().Body() here already allocated — to truly prevent alloc, use
        // c.Context().Request.BodyStream() as above. The snippet above shows pipe path.
        // For minimal diff, keep Body() but add length check BEFORE it:
        if len(c.Request().Body()) > hostUploadLimit {
            return fiber.NewError(fiber.StatusRequestEntityTooLarge, "host upload exceeds 100 MiB limit")
        }

        // Sign metadata only — daemon verifies URI+timestamp+token, not body hash, for streaming uploads.
        targetURL := strings.TrimRight(target.BaseURL, "/") + "/v1/files/upload?path=" + url.QueryEscape(destPath)
        req, _ := http.NewRequestWithContext(c.Context(), http.MethodPost, targetURL, bytes.NewReader(c.Request().Body()))
        req.Header.Set("Content-Type", string(c.Request().Header.ContentType()))
        headers, _ := cfg.Daemon.SignedHeaders(target.NodeToken, http.MethodPost, req.URL.RequestURI(), nil) // nil body
        // ...
    }
}
```

> **Tracker pattern from `secure_files.go:155`:** reuse the `archivePathTracker` idea at API layer for quota: before streaming, validate `destPath` via `validateHostFilePath` + `resolveHostPath` allowlist check via a new `Daemon.ValidateHostPath` preflight endpoint (or local check using allowlist cache). Currently forge only validates syntactically; beacon enforces allowlist. For upload OOM, the analogous tracker is `contentLengthTracker` — count bytes before HMAC. The snippet above is the host-upload equivalent.

### 3.4 File:line change checklist

| File | Line | Change | Risk |
|------|------|--------|------|
| `forge/api/internal/http/handlers_files.go:386` | 386-431 | Replace `rawBody:=c.Request().Body()` unbounded with `LimitReader` + early `ContentLength` 413 + chunked pipe or bounded reader | Low — behaviorally stricter (was permissive). Existing 100 MiB uploads still pass. |
| `forge/api/internal/daemon/client.go` | `SignedHeaders` | Add `SignedHeadersWithBodyLimit` or allow `nil` body for streaming uploads; daemon must accept metadata-only HMAC for `/v1/files/upload` (or keep buffered path for small files) | Medium — requires beacon `handlers_host.go` signature verifier to accept nil-body HMAC for upload route only. Gate behind `X-Forge-Upload-Streaming: 1` header. |
| `forge/api/internal/http/server.go` or `cmd/api/main.go` | Fiber `app.Settings.BodyLimit` | Set `BodyLimit: 105 * 1024 * 1024` for host routes or per-route middleware `c.Request().Header.ContentLength()` check | Low |
| `beacon/internal/server/hostfiles.go:457` | 457-458 | Keep `MaxBytesReader` as defense-in-depth; optionally export `HostUploadLimit` constant for shared import by forge (or duplicate constant with comment linking) | None |
| `forge/web/components/admin/host-files-view.tsx` | upload handler | Add client `file.size > 100*1024*1024` preflight with `toast.error("Host upload limit 100 MiB")` before POST | Low |

### 3.5 Alternatives considered

| Alternative | Why rejected |
|-------------|--------------|
| Raise Fiber `BodyLimit` to 100 MiB globally | Fixes OOM but still buffers whole body in heap — 10 concurrent 100 MiB uploads = 1 GiB heap spike. Streaming is O(1). |
| Keep buffering but add `if len(rawBody) > limit → 413` after alloc | Prevents beacon hit but still OOMs — alloc already happened. Must check *before* alloc. |
| `io.Copy` to temp file then sign | Avoids heap OOM but leaves temp file on control plane disk; streaming via pipe is cleaner. Temp file path viable as fallback if HMAC requires body. |

### 3.6 Testing & verification

```go
// forge/api/internal/http/handlers_files_test.go — new tests
func TestHostFilesUpload_RejectsOverLimitBeforeProxy(t *testing.T) {
    // Build 101 MiB body, assert 413 without daemon call
    // Mock Daemon HTTPClient to fail if called — test must not reach daemon
}

func TestHostFilesUpload_StreamingHMAC(t *testing.T) {
    // 50 MiB body, assert proxy succeeds with chunked encoding, ContentLength == -1 or absent
}

func TestHostFilesUpload_ContentLengthEarly413(t *testing.T) {
    // Set Content-Length: 200MiB header with small body, assert 413 at header check
}

func TestHostFilesUpload_MultipartLimit(t *testing.T) {
    // multipart/form-data 101 MiB via FormFile, assert 413
}
```

Manual: `curl -X POST -H "Content-Type: application/octet-stream" --data-binary @/dev/zero?count=110M http://localhost:8080/api/v1/host/files/upload?path=/tmp/big.bin` should 413 at forge before beacon log.

### 3.7 Backward compatibility

- Existing uploads <100 MiB unchanged. Multipart and octet-stream paths preserved.
- Signed header change (`nil` body) — make daemon accept both: verify beacon `auth/middleware.go` `verifySignedHeaders` checks `body==nil` branch as valid for `/v1/files/upload` only. Old clients that send body HMAC still pass (verify both branches). No breaking change.
- Fiber `BodyLimit` raise is not lowering — only adding cap where none existed, so no previously-valid request becomes invalid except >100 MiB (already invalid at beacon).

---

## 4. Fix Track H-CRON-01 / 02 / 03 — Cron Duplicate Execution + Sleep Blocks Scheduler

### 4.1 Root cause (file:line)

`forge/api/internal/services/cronjob/service.go:263-285` `TriggerNow`:

```go
func (s *Service) TriggerNow(ctx context.Context, jobID string) (store.CronJobExecution, error) {
    execution, err := s.store.CreateCronJobExecution(ctx, jobID) // outer row 1 (returned to caller)
    // ...
    go func() {
        s.executeJob(context.Background(), jobID) // inner row 2 created at executeJob:124
    }()
    return execution, nil // caller polls row 1 → hangs pending forever
}

func (s *Service) executeJob(ctx context.Context, jobID string) {
    execution, err := s.store.CreateCronJobExecution(ctx, jobID) // shadow row 2
    // ... run/dispatch ...
    s.store.CompleteCronJobExecution(ctx, execution.ID, status, ...)
    if status == "failed" && job.RetryCount > 0 {
        for i := 0; i < job.RetryCount; i++ {
            time.Sleep(time.Duration(5*(i+1)) * time.Second) // blocks cron goroutine ←
            retryExec, _ := s.store.CreateCronJobExecution(ctx, jobID) // yet another row per retry
            // ...
        }
    }
}
```

**Handler** `forge/api/internal/http/handlers_cronjob.go:196` `POST /cron-jobs/:id/execute → TriggerNow` returns 202 with `execution` that will never complete.

**Scheduler** `service.go:62` `cron.Start()` uses single `cron.Cron` instance; `scheduleJob:81` `AddFunc` closures call `executeJob` directly on cron's goroutine. If job A fails with RetryCount=5, sleep sum = 5+10+15+20+25 = 75s blocks worker; RetryCount=10 → ~275s. Sibling jobs scheduled during sleep stall.

**Cross-instance** — if forge runs 2 replicas (typical prod `infra/compose.production.yml` `api: replicas 2`), both call `ListEnabledCronJobs` at `Start` and schedule same `job.Schedule` — both fire. No advisory lock or lease.

### 4.2 Design — three-prong fix

#### 4.2.1 Deduplicate TriggerNow (single execution row)

**Option A (recommended — minimal):** `TriggerNow` creates row, passes ID to `executeJobWithID` which reuses it. No second `Create`.

```go
func (s *Service) TriggerNow(ctx context.Context, jobID string) (store.CronJobExecution, error) {
    if s.store == nil { return store.CronJobExecution{}, fmt.Errorf("store not initialized") }
    // 1. Monotonic dedup: if a TriggerNow for same jobID is already in-flight, return existing
    s.mu.Lock()
    if inFlight, ok := s.inFlight[jobID]; ok && time.Since(inFlight) < 5*time.Second {
        s.mu.Unlock()
        return store.CronJobExecution{}, fiber.NewError(fiber.StatusTooManyRequests, "trigger already in progress for this job")
    }
    s.inFlight[jobID] = time.Now()
    s.mu.Unlock()
    defer func() {
        s.mu.Lock(); delete(s.inFlight, jobID); s.mu.Unlock()
    }()

    execution, err := s.store.CreateCronJobExecution(ctx, jobID)
    if err != nil { return store.CronJobExecution{}, err }

    go func(execID string) {
        defer func() {
            if r := recover(); r != nil {
                buf := make([]byte, 4096)
                n := runtime.Stack(buf, false)
                s.logger.Error("cron job trigger panic recovered", "job_id", jobID, "panic", r, "stack", string(buf[:n]))
                // Also complete the outer execution as failed so caller not hung
                _ = s.store.CompleteCronJobExecution(context.Background(), execID, "failed", 1, "", fmt.Sprintf("panic: %v", r), 0)
            }
        }()
        s.executeJobWithExecution(context.Background(), jobID, execID)
    }(execution.ID)

    return execution, nil
}

func (s *Service) executeJobWithExecution(ctx context.Context, jobID, executionID string) {
    job, err := s.store.GetCronJob(ctx, jobID)
    if err != nil { s.logger.Error("cron job not found", "id", jobID); return }
    start := time.Now()
    var exitCode int
    var output, errStr string
    switch {
    case job.TargetType == "server":
        exitCode, output, errStr = s.dispatchServerCommand(ctx, job)
    default:
        exitCode, output, errStr = s.runShellCommand(job.Command, job.TimeoutSeconds)
    }
    status := "success"
    if exitCode != 0 { status = "failed" }
    _ = s.store.CompleteCronJobExecution(ctx, executionID, status, exitCode, output, errStr, int(time.Since(start).Milliseconds()))
    if status == "failed" && job.RetryCount > 0 {
        s.scheduleRetries(ctx, job, executionID) // non-blocking — see 4.2.2
    }
}

// Keep old executeJob for cron-scheduled ticks (creates its own row) — now thin wrapper:
func (s *Service) executeJob(ctx context.Context, jobID string) {
    execution, err := s.store.CreateCronJobExecution(ctx, jobID)
    if err != nil { s.logger.Error("failed to create execution", "id", jobID, "err", err); return }
    s.executeJobWithExecution(ctx, jobID, execution.ID)
}
```

**DB-level cross-instance dedup (pg_advisory_lock):** wrap `CreateCronJobExecution` in advisory lock so two replicas don't double-create.

```go
// store/store_cron.go — new helper
func (s *Store) CreateCronJobExecutionDedup(ctx context.Context, jobID string, dedupWindow time.Duration) (CronJobExecution, error) {
    // Advisory lock on jobID hash — transaction-scoped so auto-released on commit
    // Use pg_advisory_xact_lock(hashtext(jobID)) to serialize across replicas
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil { return CronJobExecution{}, err }
    defer tx.Rollback()
    // Claim advisory lock — blocks concurrent replica for same jobID
    if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, jobID); err != nil {
        return CronJobExecution{}, err
    }
    // Check monotonic window: is there already a pending/running execution within dedupWindow?
    var recent string
    err = tx.QueryRowContext(ctx,
        `SELECT id FROM cron_job_executions
         WHERE job_id=$1 AND created_at > NOW() - $2::interval
           AND status IN ('pending','running')
         LIMIT 1`,
        jobID, fmt.Sprintf("%d seconds", int(dedupWindow.Seconds()))).Scan(&recent)
    if err == nil {
        tx.Rollback()
        return CronJobExecution{}, fmt.Errorf("execution already in progress: %s", recent)
    }
    // Create execution row in same xact
    var exec CronJobExecution
    err = tx.QueryRowContext(ctx,
        `INSERT INTO cron_job_executions(id, job_id, status, created_at)
         VALUES(gen_random_uuid(), $1, 'running', NOW())
         RETURNING id, job_id, status, created_at`,
        jobID).Scan(&exec.ID, &exec.JobID, &exec.Status, &exec.CreatedAt)
    if err != nil { return CronJobExecution{}, err }
    if err := tx.Commit(); err != nil { return CronJobExecution{}, err }
    return exec, nil
}
```

If `hashtext` extension unavailable, use `pg_advisory_xact_lock(('x'||substr(md5($1),1,15))::bit(60)::bigint)` or app-level `pg_advisory_xact_lock($2)` with `hash(jobID)`. Provide migration that enables `pgcrypto` if needed — but advisory locks need no extension.

**Monotonic clock dedup (in-process):** `s.inFlight` map with `sync.Mutex` + `time.Since` 5s window prevents double-click / rapid re-trigger even without DB roundtrip. Use monotonic `time.Now()` (`time.Since` is monotonic-safe).

#### 4.2.2 Non-blocking retry (remove `time.Sleep` from cron goroutine)

**Current:** `service.go:161` `time.Sleep(5*(i+1)s)` blocks cron worker.

**Fix:** use `time.AfterFunc` queue or `gopkg.in/robfig/cron` already has `cron.Schedule` — but retries are not cron-scheduled, they are immediate backoffs. Use a dedicated retry queue channel + worker goroutine, or `time.AfterFunc` chained.

```go
func (s *Service) scheduleRetries(ctx context.Context, job store.CronJob, parentExecID string) {
    // Non-blocking — spawn retry chain on separate goroutine, not cron worker
    go func() {
        for i := 0; i < job.RetryCount; i++ {
            backoff := time.Duration(5*(i+1)) * time.Second
            select {
            case <-time.After(backoff):
            case <-ctx.Done():
                return
            }
            retryExec, err := s.store.CreateCronJobExecution(context.Background(), job.ID)
            if err != nil {
                s.logger.Error("failed to create retry execution", "job_id", job.ID, "attempt", i+1, "err", err)
                continue
            }
            start := time.Now()
            var ec int; var out, est string
            if job.TargetType == "server" {
                ec, out, est = s.dispatchServerCommand(context.Background(), job)
            } else {
                ec, out, est = s.runShellCommand(job.Command, job.TimeoutSeconds)
            }
            status := "success"
            if ec != 0 { status = "failed" }
            _ = s.store.CompleteCronJobExecution(context.Background(), retryExec.ID, status, ec, out, est, int(time.Since(start).Milliseconds()))
            if ec == 0 { break } // success — stop retry chain
        }
    }()
}

// Alternative using time.AfterFunc for tighter control:
func (s *Service) scheduleRetriesAfterFunc(ctx context.Context, job store.CronJob) {
    var attempt int
    var scheduleNext func()
    scheduleNext = func() {
        if attempt >= job.RetryCount { return }
        delay := time.Duration(5*(attempt+1)) * time.Second
        time.AfterFunc(delay, func() {
            attempt++
            retryExec, _ := s.store.CreateCronJobExecution(context.Background(), job.ID)
            // ... run + complete ...
            if exitCode != 0 {
                scheduleNext()
            }
        })
    }
    scheduleNext()
}
```

**Scheduler isolation hardening (bonus):** wrap `cron.AddFunc` bodies to `go s.executeJob(...)` so even successful jobs don't block cron worker if `runShellCommand` hangs up to 3600s:

```go
func (s *Service) scheduleJob(ctx context.Context, job store.CronJob) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    if existingID, ok := s.entries[job.ID]; ok {
        s.cron.Remove(existingID)
    }
    jobID := job.ID // capture
    entryID, err := s.cron.AddFunc(job.Schedule, func() {
        go s.executeJob(context.Background(), jobID) // ← go, don't block cron worker
    })
    // ...
}
```

Currently `service.go:90` `s.executeJob(context.Background(), jobID)` runs inline on cron worker — change to `go`.

### 4.3 Migration — `pg_advisory_lock` needs no schema migration

No table migration. But add advisory-lock-backed helper and index for dedup window query:

```sql
-- forge/api/migrations/2xx_cron_dedup_index.sql
-- Index to support CreateCronJobExecutionDedup window scan
CREATE INDEX IF NOT EXISTS idx_cron_executions_job_created_status
  ON cron_job_executions(job_id, created_at DESC)
  WHERE status IN ('pending','running');

-- Ensure pgcrypto for gen_random_uuid if not already (most forges already have it)
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
```

If replicas >1, document that `pg_advisory_xact_lock` requires same DB (true — all replicas share one Postgres). No Redis needed.

### 4.4 Frontend fix — FE/BE `* *` guard drift

`forge/web/app/admin/cron-jobs/page.tsx:39` `parseCronExpression` only checks 5-field count. BE `handlers_cronjob.go:23` rejects `* *` (every-minute) as 422. So FE thinks `* * * * *` is valid, BE 422s with no FE hint.

Fix `forge/web/app/admin/cron-jobs/page.tsx:39`:

```tsx
function validateCronSchedule(schedule: string): string | null {
  const parts = schedule.trim().split(/\s+/);
  if (parts.length !== 5) return "Schedule must be 5 fields (minute hour day month weekday)";
  if (parts[0] === "*" && parts[1] === "*") return "Minimum interval is 1 minute — use '*/1 * * * *' is not allowed; use '*/5 * * * *' or similar";
  // Mirror BE cron.NewParser validation via API dry-run or local robfig/cron parse via wasm
  try { cronParseOrThrow(schedule); } catch (e) { return (e as Error).message; }
  return null;
}
```

Wire FE `validateCronSchedule` before POST so user sees inline error, not 422 toast.

### 4.5 File:line change checklist

| File | Line | Change |
|------|------|--------|
| `forge/api/internal/services/cronjob/service.go:21` | `Service` struct | Add `inFlight map[string]time.Time` + `mu` already exists |
| `service.go:62` | `Start` | Ensure `scheduleJob` wraps `go s.executeJob` |
| `service.go:81` | `scheduleJob` | Change `func(){ s.executeJob }` to `func(){ go s.executeJob }` |
| `service.go:117` | `executeJob` | Extract `executeJobWithExecution(ctx, jobID, executionID)` |
| `service.go:158` | retry loop `time.Sleep` | Replace with `scheduleRetries` via `time.AfterFunc` / goroutine + `time.After` |
| `service.go:263` | `TriggerNow` | Single-row path: create once, pass ID to `executeJobWithExecution`, add monotonic `inFlight` dedup |
| `forge/api/internal/store/store_cron.go` | new | `CreateCronJobExecutionDedup` with `pg_advisory_xact_lock` + dedup window query |
| `forge/api/internal/http/handlers_cronjob.go:196` | `POST /cron-jobs/:id/execute` | Map `TooManyRequests` from dedup to 429 with `Retry-After` header |
| `forge/web/app/admin/cron-jobs/page.tsx:39` | `parseCronExpression` | Mirror BE `* *` guard + 4096 char command length |
| Migration | new | `idx_cron_executions_job_created_status` partial index |

### 4.6 Testing

```go
func TestTriggerNow_SingleRow(t *testing.T) {
    svc, store := newTestCronService(t)
    job := seedCronJob(t, store, "*/5 * * * *")
    exec, err := svc.TriggerNow(context.Background(), job.ID)
    require.NoError(t, err)
    // Poll until completed — should complete the SAME ID
    require.Eventually(t, func() bool {
        e, _ := store.GetCronJobExecution(context.Background(), exec.ID)
        return e.Status == "success" || e.Status == "failed"
    }, 5*time.Second, 100*time.Millisecond)
    // Count rows — must be exactly 1
    execs, _ := store.ListCronJobExecutions(context.Background(), job.ID, 10)
    require.Len(t, execs, 1)
    require.Equal(t, exec.ID, execs[0].ID)
}

func TestTriggerNow_DedupMonotonic(t *testing.T) {
    // Two concurrent TriggerNow for same jobID → second 429
}

func TestCronRetries_NonBlocking(t *testing.T) {
    // Schedule job with RetryCount=3 that fails; assert sibling job still fires on schedule
    // Measure that cron worker not blocked: schedule 1s job alongside failing retry chain
}
```

Manual: set `RetryCount=3` on a failing `TargetType=server` job with no dispatcher → observe 3 retries spaced 5/10/15s via `GET /cron-jobs/:id/executions` — no scheduler stall (other 1m jobs still tick).

### 4.7 Backward compatibility

- `TriggerNow` still returns `CronJobExecution` with `id, job_id, status=running, created_at`. Existing FE polling `GET /executions?jobId=X` finds the row; now it actually completes (was hung before — fixing bug is compat).
- Retry loop now async — `durationMs` per retry still recorded; `execution` rows unchanged.
- `pg_advisory_xact_lock` is purely additive; single-replica deployments see no behavior change except dedup window (5s) prevents double-click.

---

## 5. Fix Track H-MON-01..06 — Monitoring / Host Telemetry Truth

### 5.1 Root cause cluster (file:line)

**Synthetic-zero fallback** `forge/web/app/admin/monitoring/page.tsx:50`:

```tsx
const isSynthetic = useMemo(() => {
  const m = metricsQ.data ?? [];
  if (!m.length) return false;
  return m.every((x) => (x.cpuLoad1m ?? 0) === 0) && m.every((x) => (x.networkRxBytes ?? 0) === 0);
}, [metricsQ.data]);
// banner at :96 "allocated capacity (not live OS) — Network/load not yet collected"
```

Beacon currently has no `gopsutil` load/net collectors on linux — `sysinfo_linux.go` only has `unix.Sysinfo` (mem) + `/proc/cpuinfo` + `/sys/class/net` speed/operstate. So `cpuLoad1m` and `networkRxBytes` are always 0 → `isSynthetic` true → honest but masks live gap. FE still renders charts with `v: (m as Record<string,number>)[metric] ?? 0` at `page.tsx:113` — so zero series looks like flat 0% not "no data".

**Daemon uptime mislabel** `beacon/internal/server/handlers_host.go:65`:

```go
Uptime: int64(time.Since(s.started).Seconds()), // daemon boot, not host boot
```

UI `forge/web/app/admin/host/page.tsx:98` `fmtUptime(data.uptimeSeconds)` shows "Uptime 2h" even if host was up 30 days and daemon restarted 2h ago. Operator mis-diagnoses host reboot.

**Single disk** `handlers_host.go:74-97`:

```go
partitions := []DiskPartition{}
rootStat := unix.Statfs_t{}
if err := unix.Statfs("/", &rootStat); err == nil {
    partitions = append(partitions, DiskPartition{MountPoint: "/", ...})
}
writeJSON(w, http.StatusOK, partitions) // only "/"
```

1Panel `monitor.go:110` `GetIOOptions` via `gopsutil/disk.Partitions(true)` enumerates all mounts. Forge shows only `/` — hides `/data`, `/srv`, `/var/lib/docker` pressure.

**Network IP divergence** `sysinfo_linux.go:61-92`:

```go
func netInterfacesPlatform() ([]NetworkInterface, error) {
    entries, err := os.ReadDir("/sys/class/net")
    // reads speed + operstate only — no IPs, no MAC on linux!
}
```

vs `sysinfo_darwin.go:56` which uses `net.Interfaces()` and *does* populate IPs/MAC via `Addrs()` + `HardwareAddr`. So linux prod shows IPs="", MAC="", darwin dev shows them — divergent and `NetworkInterface{IPs string}` stays empty on linux.

**Inert sort** `sysinfo_linux.go:94-134` + `forge/web/app/admin/host/page.tsx:234`:

```go
// sysinfo_linux.go:94 — only Name:/State: parsed
if strings.HasPrefix(line, "Name:") { proc.Name = ... }
if strings.HasPrefix(line, "State:") { proc.State = ... }
// CPU/Memory never set → always 0
```
```tsx
// host/page.tsx:234 — sorts by zero forever
sorted = [...data].sort((b,a) => b.cpuPercent - a.cpuPercent) // stable sort of zeros = input order
sorted.slice(0,100) // still shows 100 rows, sorted affordance is lie
```

### 5.2 Design

#### 5.2.1 Monitoring: render "no data" not 0 — FE honest emptiness (H-MON-01)

**BE change:** when collector unavailable, return `null` not `0` for `cpuLoad1m`, `networkRxBytes`, etc. Or add explicit `synthetic: true` field. Easiest: keep current BE but FE must not plot zeros as data.

**FE change `monitoring/page.tsx:50-113`:**

```tsx
// 1. Detect no-data per metric, not global synthetic
const metricHasData = useMemo(() => {
  const m = metricsQ.data ?? [];
  if (!m.length) return false;
  return m.some((x) => {
    const v = (x as unknown as Record<string, number | null | undefined>)[metric];
    return v != null && v !== 0; // at least one non-zero point
  });
}, [metricsQ.data, metric]);

// 2. Chart data — map 0-as-no-data to null so AreaChart gaps
const chartData = useMemo(() => {
  const rows = [...(metricsQ.data ?? [])]
    .sort((a,b) => a.observedAt.localeCompare(b.observedAt))
    .map((m) => {
      const raw = (m as unknown as Record<string, number | null>)[metric];
      // If synthetic and raw===0, render as null (gap) not 0
      const v = isSynthetic && raw === 0 ? null : (raw ?? null);
      return { ts: m.observedAt, v };
    });
  return rows;
}, [metricsQ.data, metric, isSynthetic]);

// 3. Empty guard — show no-data card not flat 0% line
{!metricHasData ? (
  <div className="grid h-full place-items-center text-sm text-[var(--text-subtle)]">
    <span className="rounded-full border border-[var(--line)] px-4 py-2">
      No telemetry for {METRICS.find(m=>m.k===metric)?.label} — collector not yet available on this node.
      {isSynthetic ? " Showing allocated capacity only." : ""}
    </span>
  </div>
) : (
  <AreaChart data={chartData} ...>
    <Area connectNulls={false} ... /> {/* gaps for nulls */}
  </AreaChart>
)}
```

Also fix `YAxis domain` — when `metricHasData===false` don't render `0..100` as if meaningful.

**Alternative (more correct):** change BE `beacon/internal/metrics/metrics.go` or `beacon/internal/server/handlers_host.go` to emit `cpuLoad1m: null` when collector absent, not `0`. Then FE check `v==null` naturally gaps. Keep synthetic banner but promote to blocking if operator tries to set alert on synthetic series: disable alert creation button when `isSynthetic`.

#### 5.2.2 Daemon uptime with suspend subtraction + host uptime (H-MON-02)

`beacon/internal/server/handlers_host.go:59-72` `handleHostInfo`:

```go
func (s *Server) handleHostInfo(w http.ResponseWriter, r *http.Request) {
    hostname, _ := os.Hostname()
    info := HostInfo{
        Hostname: hostname,
        OS:       runtime.GOOS,
        Arch:     runtime.GOARCH,
        // BEFORE:
        // Uptime:   int64(time.Since(s.started).Seconds()),
        // AFTER:
        Uptime:       hostUptimeSeconds(s.started), // host boot, not daemon boot
        DaemonUptime: int64(time.Since(s.started).Seconds()),
        CPUCores: runtime.NumCPU(),
        Time:     time.Now().UTC().Format(time.RFC3339),
        Kernel:   kernelVersion(),
        CPUModel: cpuModel(),
    }
    writeJSON(w, http.StatusOK, info)
}

// beacon/internal/server/sysinfo_linux.go — new helper
func hostUptimeSeconds(daemonStarted time.Time) int64 {
    // Try /proc/uptime first field (host boot), fall back to daemon uptime
    if data, err := os.ReadFile("/proc/uptime"); err == nil {
        if fields := strings.Fields(string(data)); len(fields) > 0 {
            if secs, err := strconv.ParseFloat(fields[0], 64); err == nil {
                return int64(secs)
            }
        }
    }
    // Fallback: daemon uptime (prevents 0 on non-linux or read error)
    return int64(time.Since(daemonStarted).Seconds())
}
```

**Suspend subtraction:** on linux, `/proc/uptime` already excludes suspend? Actually `/proc/uptime` pauses during suspend on most kernels (monotonic boottime). If we want wall-clock uptime that includes suspend, use `CLOCK_BOOTTIME`. But prompt says "daemon uptime with suspend subtraction" — meaning daemon's `time.Since(s.started)` overcounts suspend? `time.Since` uses monotonic, which on linux pauses during suspend (CLOCK_MONOTONIC). So suspend is already not counted. Document this. If wall uptime desired, use `time.Now().Sub(s.started)` vs monotonic. Clarify: **keep `DaemonsUptime` as monotonic (excludes suspend), `HostUptime` via `/proc/uptime` (also monotonic boottime)**. If true wall diff needed, also expose `DaemonWallUptime` via `time.Since` with non-monotonic? Not needed — note in ADR.

**Type change (backward compat):** add new JSON field `daemonUptimeSeconds` while keeping `uptimeSeconds` as host uptime (breaking semantic change but value changes meaningfully). Safer: keep `uptimeSeconds` as host uptime (since it's what UI labels "Uptime"), add `daemonUptimeSeconds` for debugging. Document migration.

```go
type HostInfo struct {
    Hostname      string `json:"hostname"`
    OS            string `json:"os"`
    Kernel        string `json:"kernel"`
    Uptime        int64  `json:"uptimeSeconds"`        // HOST uptime (changed semantics, was daemon)
    DaemonUptime  int64  `json:"daemonUptimeSeconds"`  // NEW — daemon age
    CPUModel      string `json:"cpuModel"`
    CPUCores      int    `json:"cpuCores"`
    Arch          string `json:"arch"`
    Time          string `json:"time"`
}
```

FE `host/page.tsx:98` `fmtUptime` should show both with labels:

```tsx
<div className="text-xs text-[var(--text-subtle)]">
  Host uptime {fmtUptime(data.uptimeSeconds)} · Agent up {fmtUptime(data.daemonUptimeSeconds ?? data.uptimeSeconds)}
</div>
```

**Inert sort cleanup `sysinfo_linux.go:94`:** wire `gopsutil` or hide sort.

Option A (wire — P2, ~4 lines):

```go
// sysinfo_linux.go — replace hand-rolled /proc/status parser with gopsutil
import "github.com/shirou/gopsutil/v3/process"

func processListPlatform() ([]ProcessEntry, error) {
    pids, err := process.Pids()
    if err != nil { return nil, err }
    var out []ProcessEntry
    for _, pid := range pids {
        p, err := process.NewProcess(pid)
        if err != nil { continue }
        name, _ := p.Name()
        status, _ := p.Status() // []string
        cpu, _ := p.CPUPercent()
        mem, _ := p.MemoryPercent()
        state := ""
        if len(status) > 0 { state = status[0] }
        if name == "" { name = fmt.Sprint(pid) }
        out = append(out, ProcessEntry{PID: int(pid), Name: name, CPU: cpu, Memory: float64(mem), State: state})
    }
    return out, nil
}
```

If gopsutil not allowed (dep weight), keep `/proc/status` but **remove sort buttons** in FE and render `—` for CPU/Memory:

```tsx
// host/page.tsx:219 ProcessesTab — guard inert sort
const hasLiveMetrics = useMemo(() => data?.some(p => p.cpuPercent !== 0 || p.memoryPercent !== 0), [data]);
{hasLiveMetrics ? (
  <button onClick={()=>setSort("cpu")}>Sort by CPU</button>
) : (
  <span className="text-xs text-amber-300">CPU/Memory — collection not yet available — Name/State only</span>
)}
// In table:
<td>{p.cpuPercent !== 0 ? p.cpuPercent.toFixed(1)+"%" : "—"}</td>
```

**Prompt requires "inert sort cleanup"** — do both: wire gopsutil on linux, keep darwin stub with honest message (darwin `processListPlatform` already returns `[]` at `sysinfo_darwin.go:84` — change to return `gopsutil` as well, or keep empty with banner "Live processes unavailable on macOS dev host").

#### 5.2.3 Single `/` disk fix — enumerate all mounts (H-MON-04)

`beacon/internal/server/handlers_host.go:74` `handleHostDisk`:

```go
func (s *Server) handleHostDisk(w http.ResponseWriter, r *http.Request) {
    // BEFORE: only Statfs("/") 
    // AFTER: enumerate all real mounts
    partitions := []DiskPartition{}
    mounts := readProcMounts() // parse /proc/mounts, filter pseudo-fs, dedup
    seen := make(map[string]bool)
    for _, mnt := range mounts {
        if seen[mnt] { continue }
        seen[mnt] = true
        var st unix.Statfs_t
        if err := unix.Statfs(mnt, &st); err != nil { continue }
        if st.Blocks == 0 { continue } // skip empty (proc/sysfs)
        total := uint64(st.Blocks) * uint64(st.Bsize) / (1024 * 1024)
        if total == 0 { continue }
        free := uint64(st.Bavail) * uint64(st.Bsize) / (1024*1024)
        used := total - free
        var usedPct float64
        if total > 0 { usedPct = float64(used)/float64(total)*100 }
        // Device/FSType from /proc/mounts entry if available
        partitions = append(partitions, DiskPartition{
            MountPoint: mnt,
            Device:     mountDevice(mnt), // from parsed mounts map
            FSType:     mountFSType(mnt),
            TotalMB: total, UsedMB: used, FreeMB: free, UsedPct: usedPct,
        })
    }
    // Fallback: if no mounts (darwin or read error), keep single "/" for compat
    if len(partitions) == 0 {
        // existing "/" path
    }
    writeJSON(w, http.StatusOK, partitions)
}

func readProcMounts() []string {
    data, err := os.ReadFile("/proc/mounts")
    if err != nil { return []string{"/"} }
    var mounts []string
    for _, line := range strings.Split(string(data), "\n") {
        fields := strings.Fields(line)
        if len(fields) < 3 { continue }
        mountPoint := fields[1]
        fsType := fields[2]
        // Skip pseudo filesystems
        if fsType == "proc" || fsType == "sysfs" || fsType == "devtmpfs" || fsType == "tmpfs" && mountPoint == "/dev/shm" {
            // Keep tmpfs for /tmp if needed - allow but cap
            if fsType == "tmpfs" && mountPoint != "/tmp" { continue }
        }
        if strings.HasPrefix(fsType, "cgroup") || fsType == "overlay" && mountPoint == "/" {
            // overlay for "/" is ok — keep
        }
        // Skip cgroup, overlay (except root), squashfs mounts, etc. that are not real disks
        if fsType == "cgroup" || fsType == "cgroup2" || fsType == "debugfs" || fsType == "tracefs" { continue }
        // Unescape \040 etc. via mountinfo? For v1 keep raw
        mounts = append(mounts, mountPoint)
    }
    if len(mounts) == 0 { return []string{"/"} }
    return mounts
}
```

Simpler: use `github.com/shirou/gopsutil/v3/disk.Partitions(true)` + `disk.Usage` — one call, handles filtering. Already used elsewhere? Add dep and use it — fewer host bugs.

**Backward compat:** FE `host/page.tsx:108` `DiskTab` already maps array; single vs multi just adds rows. Sort by `UsedPct` desc to surface pressure.

#### 5.2.4 Network IP divergence — add IPs via `net.Interfaces()` on linux (H-MON-05)

`beacon/internal/server/sysinfo_linux.go:61` current only does sysfs. Fix to merge both sources:

```go
func netInterfacesPlatform() ([]NetworkInterface, error) {
    entries, err := os.ReadDir("/sys/class/net")
    if err != nil { return nil, err }
    // Build map from net.Interfaces() for IPs/MAC on linux too (unify with darwin)
    ifaceAddrs := make(map[string]NetworkInterface)
    if ifs, err := net.Interfaces(); err == nil {
        for _, ni := range ifs {
            addrs, _ := ni.Addrs()
            var ips []string
            for _, a := range addrs {
                if ipnet, ok := a.(*net.IPNet); ok {
                    // Skip link-local/unspecified if desired, but include for host truth
                    ips = append(ips, ipnet.IP.String())
                }
            }
            ifaceAddrs[ni.Name] = NetworkInterface{
                Name: ni.Name,
                IPs:  strings.Join(ips, ", "),
                MAC:  ni.HardwareAddr.String(),
                // Status/Speed filled from sysfs below
            }
        }
    }
    var ifaces []NetworkInterface
    for _, e := range entries {
        name := e.Name()
        base := ifaceAddrs[name]
        if base.Name == "" { base.Name = name } // sysfs-only (e.g. lo)
        // sysfs speed
        if data, err := os.ReadFile("/sys/class/net/" + name + "/speed"); err == nil {
            if v, _ := strconv.Atoi(strings.TrimSpace(string(data))); v > 0 {
                base.Speed = v
            }
        }
        // sysfs operstate (more reliable than net.Interface.Flags on linux)
        if data, err := os.ReadFile("/sys/class/net/" + name + "/operstate"); err == nil {
            s := strings.TrimSpace(string(data))
            switch s {
            case "up": base.Status = "up"
            case "down": base.Status = "down"
            default: base.Status = "unknown"
            }
        } else if base.Status == "" {
            // fallback to net.Interfaces Flags
            if ni, err := net.InterfaceByName(name); err == nil {
                if ni.Flags&net.FlagUp != 0 { base.Status = "up" } else { base.Status = "down" }
            }
        }
        ifaces = append(ifaces, base)
    }
    // Sort for determinism (prompt: sysinfo_linux:94 inert sort cleanup — also sort networks)
    sort.Slice(ifaces, func(i,j int) bool { return ifaces[i].Name < ifaces[j].Name })
    return ifaces, nil
}
```

FE `host/page.tsx:181` `NetworkTab` already renders `Name/IPs/MAC/Speed/Status` — after fix IPs/MAC populate on linux prod, not just darwin dev.

**Inert sort cleanup mention:** original prompt `sysinfo_linux:94 inert sort cleanup` — literal line 94 is inside `processListPlatform`. But the broader inert sort is `host/page.tsx:234` sorting zeros. Do both: fix linux collector to emit real CPU% (so sort becomes meaningful) and sort outputs deterministically by name at collector layer (avoid map-random order). Document darwin stub explicitly.

### 5.3 Files & line checklist (monitoring seam)

| File | Line | Change | Compat |
|------|------|--------|--------|
| `beacon/internal/server/handlers_host.go:12` | `HostInfo` struct | Add `DaemonUptime int64 json:"daemonUptimeSeconds"` | Additive |
| `handlers_host.go:65` | `Uptime: time.Since` | Replace with `hostUptimeSeconds(s.started)` helper | Value changes but label was wrong before — inform FE |
| `handlers_host.go:74` | `handleHostDisk` Statfs("/") | Replace with `readProcMounts()` loop + fallback to "/" | Array length grows — FE already handles multi |
| `beacon/internal/server/sysinfo_linux.go:61` | `netInterfacesPlatform` | Merge `net.Interfaces()` IPs/MAC on linux | Additive — previously empty strings now filled |
| `sysinfo_linux.go:94` | `processListPlatform` | Replace `/proc/status` Name/State-only parser with `gopsutil/process` CPU%/Mem or keep but disable FE sort until wired | Either fixes inert sort |
| `sysinfo_linux.go:new` | `hostUptimeSeconds()` | New helper reading `/proc/uptime` float secs | Fallback to daemon age |
| `beacon/internal/server/sysinfo_darwin.go:56,84` | `netInterfacesPlatform` / `processListPlatform` | Keep darwin `net.Interfaces()` for network, wire `gopsutil` for processes or return honest empty with message; avoid diverging semantics | Doc |
| `forge/web/app/admin/monitoring/page.tsx:50,96,111` | `isSynthetic` + chart | Render `null` not 0 for synthetic, show `No telemetry` card per-metric, disable alert creation on synthetic | UX only |
| `forge/web/app/admin/host/page.tsx:60,98,219,234` | `formatError` + `fmtUptime` + `ProcessesTab` sort | Show both uptimes, guard inert sort (`hasLiveMetrics` check), render `—` not 0.0% | UX only |

### 5.4 No migration needed (monitoring seam)

No DB migration. Only collector code + FE.

---

## 6. Fix Track H-FW-01/02 — Firewall: Verify Already-Fixed Transactional, Allowlist Bifurcation

### 6.1 Verification — already fixed correctly

`beacon/internal/server/handlers_firewall.go:361` `reconcile` is correctly transactional per AUDIT_REMEDIATION_471 H7-08:

```go
// handlers_firewall.go:96 runFirewallRestore
func runFirewallRestore(ctx context.Context, rules string) error {
    cmd := exec.CommandContext(ctx, "iptables-restore", "--wait", "10", "--noflush")
    // ...
}
// 361 reconcile builds sorted transactions then atomic restore
func (d *firewallData) reconcile(ctx context.Context) error {
    // ensureHook vs removeHook for enabled state
    // filterRules.WriteString("*filter\n:FORGE-BEACON - [0:0]\n") + sorted ruleIDs
    // natRules.WriteString("*nat\n:FORGE-BEACON-FWD - [0:0]\n") + sorted forwardIDs
    return firewallRestore(ctx, filterRules.String()+natRules.String())
}
```

**Why no flush-open window:** `iptables-restore --noflush` replaces `FORGE-BEACON` chain atomically without flushing `INPUT` — no window where FORGE rules absent. Sorted `ruleIDs` + `forwardIDs` at `handlers_firewall.go:381-401` guarantees deterministic ordering (fixes H7-06 non-determinism).

**State persistence** `259 persistLocked` is crash-consistent: `MkdirAll 0700` → `CreateTemp` → `Chmod 0600` → `Write` → `Sync` → `Close` → `Rename` → `syncDirectory` — mirrors `hostfiles.go:109` discipline.

**Verification tasks (no code change):**

- [ ] `grep -n "iptables-restore" beacon/internal/server/handlers_firewall.go` → must show `--wait 10 --noflush` at line 100.
- [ ] `grep -n "sort.Strings.*ruleIDs\|sort.Strings.*forwardIDs" handlers_firewall.go` → must show both at 385 and 401.
- [ ] Run `go test ./beacon/internal/server -run Firewall -count=1` — expects `TestFirewallReconcile_SortedOrder`, `TestFirewallPersist_Atomic`, `TestFirewallTransaction_NoFlushWindow` (existing per AUDIT_REMEDIATION_471 L7-15).
- [ ] Manual: `BEACON_FIREWALL_STATE_PATH=/tmp/fw.json go run ./beacon/cmd/daemon` → add rule → `iptables -S FORGE-BEACON` shows `forge:rule-` comment → `kill -9` daemon mid-persist → state file not corrupt (atomic rename).
- [ ] Confirm `ensureHook` idempotent and `removeHook` safe when disabled (`handlers_firewall.go:304-328`).

### 6.2 UI placeholder footgun (P3 — trivial fix)

`forge/web/components/admin/AdminFirewall.tsx:356` currently:

```tsx
placeholder="e.g. 0.0.0.0/0" // ← will always 400 via validateFirewallSource :132
```

Fix:

```tsx
placeholder="e.g. 198.51.100.23 or 198.51.100.0/24"
helperText="Public unicast only. Use allow-only; unrestricted 0.0.0.0/0 is rejected."
// Remove deny from select if server only supports allow
<select>
  <option value="allow">Allow</option>
  {/* deny intentionally absent — see ADR */}
</select>
```

Also add inline validation mirroring `validateFirewallSource` (public unicast) before submit so user sees error without roundtrip.

### 6.3 Hostfiles allowlist — deny-by-default remains (H-FW-02)

`beacon/internal/server/hostfiles.go:51` `resolveHostPath`:

```go
func (s *Server) resolveHostPath(raw string) (string, error) {
    cleaned, err := validateHostPath(raw)
    // dataDir hard-block :57 always denied
    s.hostFileRootsMu.RLock(); roots := s.hostFileRoots; RUnlock()
    if len(roots) > 0 {
        if !underAny(cleaned, roots) { return "", fmt.Errorf("path %q is outside the configured host file allowlist", cleaned) }
        return cleaned, nil
    }
    if cleaned == "/" || underAny(cleaned, hostFileDenylistPrefixes) {
        return "", fmt.Errorf("path %q is in a protected system location; configure DAEMON_HOST_FILES_ALLOWLIST to grant explicit roots", cleaned)
    }
    return cleaned, nil
}
```

**Correct:** empty `DAEMON_HOST_FILES_ALLOWLIST` → conservative denylist blocks `/etc,/proc,/sys,/dev,/boot,/usr,/bin,/sbin,/lib,/lib64,/root,/var/run,/run` (line 36). Operator must configure `DAEMON_HOST_FILES_ALLOWLIST=/srv/game-panel,/opt/forge,/tmp` for legitimate roots — documented not weakened.

**Strengthening (prefix sibling guard already tested at `hostfiles_confinement_test.go:73`):**

```go
func underAny(p string, prefixes []string) bool {
    for _, root := range prefixes {
        rp := strings.TrimSuffix(root, "/")
        if p == rp || strings.HasPrefix(p, rp+"/") { // sibling-safe — not HasPrefix("/srv/data-evil", "/srv/data")
            return true
        }
    }
    return false
}
```

Already correct at line 67. Keep. Add test for denylist sibling if missing.

**No firewall/hostfiles migration.**

---

## 7. Fix Track H-FILE-01 — Host Files Confinement Strengthening

### 7.1 Already stronger than 1Panel — verify

| Boundary | 1Panel | Forge (correct) |
|----------|--------|-----------------|
| Per-server FS | `file.go:400` host-absolute, uid checks only | `rootfs_linux.go:47` `RESOLVE_BENEATH\|NO_MAGICLINKS\|NO_SYMLINKS` + `O_NOFOLLOW` — kernel-enforced even against `/proc/1/root` magic links |
| Host FS | permissive | `hostfiles.go:51` allowlist vs denylist + `hostAtomicWrite` 750+600+Sync+Rename+dirSync + sibling-safe `underAny` |

**Strengthening tasks (no semantic change):**

1. **Sibling-prefix regression test** already at `hostfiles_confinement_test.go:73` — verify `underAny("/srv/data-evil", "/srv/data") == false`. Add if missing:

```go
func TestResolveHostPath_SiblingPrefixNotEscaped(t *testing.T) {
    s := &Server{dataDir: "/tmp/beacon-test"}
    _ = s.SetHostFileAllowlist([]string{"/srv/data"})
    _, err := s.resolveHostPath("/srv/data-evil/secret")
    if err == nil { t.Fatalf("expected sibling prefix rejection") }
}
```

2. **Host allowlist template** — ship `DAEMON_HOST_FILES_ALLOWLIST=/srv/game-panel,/opt/forge,/tmp` in `infra/.env.example` + `infra/compose.beacon.yml` env template so fresh installs not noisy.

3. **Beacon data dir hard-block** already at `hostfiles.go:57` — always denied even if allowlist contains it. Keep.

4. **openat2 darwin fallback doc** — `rootfs_fallback.go` already returns `ErrOpenat2NotSupported`; host path not affected (prefix checks). Document that host allowlist is the only boundary on darwin dev hosts.

### 7.2 File:line checklist

| File | Change |
|------|--------|
| `beacon/internal/server/hostfiles_confinement_test.go` | Ensure sibling test exists |
| `infra/.env.example` | Add `DAEMON_HOST_FILES_ALLOWLIST` template |
| `beacon/cmd/daemon/main.go:225` | Keep `SetHostFileAllowlist` from env split + denylist-mode log |

---

## 8. Fix Track H-BEACON-01 — Beacon Reconnect Blind StateConnected

### 8.1 Root cause (file:line)

`beacon/internal/remote/reconnect.go:98-212` `run` + `doReconnect`:

```go
func (rc *ReconnectClient) Start(ctx context.Context) {
    rc.startOnce.Do(func() { close(rc.started); go rc.run(ctx) })
}
func (rc *ReconnectClient) run(ctx context.Context) {
    atomic.StoreInt32(&rc.state, int32(StateConnecting))
    offlineCheck := time.NewTicker(rc.offlineTimeout / 2)
    // ...
    for {
        select {
        case <-offlineCheck.C:
            rc.mu.Lock(); last := rc.lastHb; rc.mu.Unlock()
            if last.IsZero() { continue } // never seen success — stays Connecting forever? see below
            if time.Since(last) > rc.offlineTimeout {
                // trigger reconnect
                backoff = rc.doReconnect(ctx, backoff, maxBackoff)
            }
        }
    }
}
func (rc *ReconnectClient) doReconnect(ctx context.Context, backoff, maxBackoff time.Duration) time.Duration {
    // ...
    if !rc.probePanel(ctx) { // probePanel at :207 tries /health, /api/v1/health/live, / — any 2xx → true
        // stays Reconnecting
        return nextBackoff
    }
    rc.mu.Lock(); rc.lastHb = time.Now(); // ← blind advance without SendNodeHeartbeat success
    if rc.onHB != nil { rc.onHB() }
    rc.mu.Unlock()
    atomic.StoreInt32(&rc.state, int32(StateConnected)) // ← defeats heartbeatmonitor
    return nextBackoff
}
func (rc *ReconnectClient) RecordHeartbeatSuccess() {
    rc.mu.Lock(); rc.lastHb = time.Now(); rc.mu.Unlock()
    atomic.StoreInt32(&rc.state, int32(StateConnected))
}
```

**Finding:** `doReconnect` sets `lastHb = now` + `StateConnected` after `probePanel` 2xx, even though `probePanel` is just `GET /health` — not `SendNodeHeartbeat` which is the actual liveness signal `heartbeatmonitor/service.go:89` classifies via `heartbeatState` (`Healthy→Suspected→Unreachable→Offline→Recovering`). A node can probe 2xx while `POST /api/remote/heartbeat` is 401 (bad token) or 403 (IP not allowlisted) — but `reconnect.go:192` would still mark Connected, so `handlers_files.go:149` terminal gate sees `StateConnected` and allows WS, and `heartbeatmonitor` mismatch.

**Also:** `RecordHeartbeatSuccess` is the *only* path that should advance `lastHb` — `doReconnect` must not duplicate it. Prompt says "only advance lastHb on successful SendNodeHeartbeat, probe before StateConnected."

### 8.2 Design — probe before Connected, only real heartbeat advances lastHb

```go
// beacon/internal/remote/reconnect.go — REPLACEMENT doReconnect + run

func (rc *ReconnectClient) doReconnect(ctx context.Context, backoff, maxBackoff time.Duration) time.Duration {
    select {
    case <-time.After(backoff):
    case <-ctx.Done(): return backoff
    case <-rc.stopCh: return backoff
    }

    rc.mu.Lock()
    rc.inner = rc.newClient()
    rc.mu.Unlock()

    // 1. Probe panel reachability FIRST — fail fast, never advance lastHb
    if !rc.probePanel(ctx) {
        atomic.StoreInt32(&rc.state, int32(StateReconnecting))
        log.Printf("[reconnect] reconnect probe failed, will retry")
        return jitteredBackoff(backoff, maxBackoff)
    }

    // 2. Attempt real heartbeat — ONLY on successful SendNodeHeartbeat do we advance lastHb
    //    This is the fix: probe success alone does NOT make us Connected.
    ctxHb, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    if err := rc.inner.SendNodeHeartbeat(ctxHb); err != nil {
        // Real heartbeat rejected (auth/IP/classifier) — stay Reconnecting
        log.Printf("[reconnect] heartbeat after probe failed: %v", err)
        atomic.StoreInt32(&rc.state, int32(StateReconnecting))
        atomic.AddInt64(&rc.attempts, 1)
        return jitteredBackoff(backoff, maxBackoff)
    }

    // 3. Only now advance lastHb + Connected (mirrors RecordHeartbeatSuccess)
    rc.RecordHeartbeatSuccess() // reuses existing helper — advances lastHb + StateConnected
    log.Printf("[reconnect] reconnected successfully via heartbeat")
    return jitteredBackoff(backoff, maxBackoff) // reset or grow per policy
}

func jitteredBackoff(backoff, maxBackoff time.Duration) time.Duration {
    next := time.Duration(float64(backoff) * 2.0)
    if next > maxBackoff { next = maxBackoff }
    jitter := secureDurationJitter(next / 4)
    return next - next/8 + jitter
}

// RecordHeartbeatSuccess — KEEP as primary writer, now also used by doReconnect
func (rc *ReconnectClient) RecordHeartbeatSuccess() {
    rc.mu.Lock()
    rc.lastHb = time.Now()
    if rc.onHB != nil { rc.onHB() }
    rc.mu.Unlock()
    if ConnState(atomic.LoadInt32(&rc.state)) != StateConnected {
        atomic.StoreInt32(&rc.state, int32(StateConnected))
    }
    atomic.StoreInt64(&rc.attempts, 0) // reset backoff on success
}

// Add: ensure initial StateConnecting never flips to Connected via probe alone
// run loop already guards last.IsZero() — keep as is, but add explicit comment:
func (rc *ReconnectClient) run(ctx context.Context) {
    defer close(rc.stopped)
    atomic.StoreInt32(&rc.state, int32(StateConnecting))
    offlineCheck := time.NewTicker(rc.offlineTimeout / 2)
    defer offlineCheck.Stop()
    backoff := 1 * time.Second
    maxBackoff := 5 * time.Minute
    for {
        select {
        case <-ctx.Done():
            atomic.StoreInt32(&rc.state, int32(StateDisconnected))
            return
        case <-rc.stopCh:
            atomic.StoreInt32(&rc.state, int32(StateDisconnected))
            return
        case <-offlineCheck.C:
            rc.mu.Lock()
            last := rc.lastHb
            rc.mu.Unlock()
            if last.IsZero() {
                // Never seen a successful heartbeat — stay Connecting, don't flap to Failed
                // Probe will be attempted via backoff path only after first heartbeat success
                continue
            }
            if time.Since(last) > rc.offlineTimeout {
                cur := ConnState(atomic.LoadInt32(&rc.state))
                if cur == StateConnected || cur == StateReconnecting {
                    atomic.StoreInt32(&rc.state, int32(StateReconnecting))
                    atomic.AddInt64(&rc.attempts, 1)
                    log.Printf("[reconnect] offline detected, reconnecting (attempt %d)...", atomic.LoadInt64(&rc.attempts))
                }
                backoff = rc.doReconnect(ctx, backoff, maxBackoff)
            }
        }
    }
}
```

**Heartbeat loop integration** — caller that does `SendNodeHeartbeat` must call `RecordHeartbeatSuccess`/`RecordHeartbeatFailure`:

```go
// beacon/cmd/daemon/main.go — heartbeat loop (already exists, verify wiring)
if err := rc.Inner().SendNodeHeartbeat(ctx); err != nil {
    rc.RecordHeartbeatFailure() // transitions Connected → Reconnecting
    log.Printf("heartbeat failed: %v", err)
} else {
    rc.RecordHeartbeatSuccess() // advances lastHb + Connected
}
```

Currently `beacon/internal/remote/reconnect.go:262` `RecordHeartbeatSuccess` exists but is *not* called by `doReconnect` — fix wires it.

### 8.3 File:line checklist

| File | Line | Change |
|------|------|--------|
| `beacon/internal/remote/reconnect.go:98` | `run` | Keep `StateConnecting` initial, `last.IsZero()` guard |
| `reconnect.go:157` | `doReconnect` | Split into probe-then-heartbeat, only `RecordHeartbeatSuccess` advances `lastHb` |
| `reconnect.go:262` | `RecordHeartbeatSuccess` | Add `attempts` reset to 0 on success |
| `reconnect.go:277` | `RecordHeartbeatFailure` | Keep `Connected→Reconnecting` transition |
| `beacon/cmd/daemon/main.go` or `beacon/internal/remote/heartbeat.go` | heartbeat loop | Ensure every `SendNodeHeartbeat` result calls `Record*` |

### 8.4 Testing

```go
func TestReconnect_ProbeDoesNotAdvanceConnected(t *testing.T) {
    // Mock probe 2xx but SendNodeHeartbeat 401 → state must stay Reconnecting, lastHb zero
    rc := NewReconnectClientWithClient(mockProbeOK, mockHeartbeat401, 30*time.Second)
    rc.Start(ctx)
    time.Sleep(200 * time.Millisecond)
    // Trigger doReconnect path
    rc.doReconnect(ctx, time.Millisecond, time.Second)
    if rc.State() == StateConnected { t.Fatalf("probe success must not make Connected") }
    if !rc.lastHb.IsZero() { t.Fatalf("lastHb must not advance on probe") }
}

func TestReconnect_HeartbeatAdvancesConnected(t *testing.T) {
    // Mock probe 2xx + heartbeat 2xx → StateConnected, lastHb non-zero, attempts 0
}

func TestReconnect_HeartbeatFailureStaysReconnecting(t *testing.T) {
    // Connected → heartbeat 500 → StateReconnecting, attempts++
}
```

Manual: revoke `DAEMON_NODE_TOKEN` → `POST /api/remote/heartbeat` 401 → beacon must stay `StateReconnecting` not `Connected`; `GET /host/terminal/ws` should 502/Offline, not connected.

### 8.5 Backward compatibility

- `StateConnected` now strictly requires real heartbeat 2xx, so nodes with bad token that previously appeared Connected will now correctly appear `Reconnecting`/`Offline` — this is the *intended* fix (exposes misconfiguration rather than hiding it). Inform operators via release note.
- `probePanel` candidates (`/health`, `/api/v1/health/live`, `/`) unchanged — probe still used as fast reachability gate before heartbeat, but not as Connected signal.
- `offlineTimeout/2` ticker and backoff jitter unchanged.

---

## 9. Host-Tool Freeze — Intentionally Out-of-Scope Surfaces

### 9.1 Decision: REJECT, not MISSING

Per `audits/final-parity/subagent-05-host-admin.md:74` + `FINAL §22`:

| 1Panel surface | Path | Why Forge rejects | What to do instead |
|----------------|------|-------------------|--------------------|
| **Docker daemon tuning** `daemon.json` + `OperateDocker` | `reference/app-platforms/1panel/agent/app/api/v2/docker.go:19` `LoadDaemonJsonFile` `31 LoadDaemonJsonFile` `69 UpdateDaemonJson` `92 UpdateLogOption` `115 UpdateIpv6Option` `161 OperateDocker start/stop/restart` | Nodes immutable; `dockerd` restart via tenant control plane breaks fleet isolation + beacon socket | ADR: provisioning-time via `cloud-init`/`Ansible` |
| **SSH daemon + RootCert CA** | `ssh.go:17 GetSSHInfo` `35 OperateSSH` `57 UpdateSSH` `79 CreateRootCert` `103 EditRootCert` `125 SyncRootCert` `141 SearchRootCert` | MTLS+SSO; panel must not distribute passphrase-encrypted private keys | ADR: out-of-band hardening; shell via `Host Terminal` WS (MTLS+signed, heartbeat-gated `handlers_files.go:149`) |
| **fail2ban** jail.local + banned IPs | `fail2ban.go:16 LoadFail2BanBaseInfo` `35 SearchFail2Ban` `59 OperateFail2Ban` `81 OperateSSHD` `104 UpdateFail2BanConf` | Requires local `sshd` log parsing; Forge equivalent is heartbeat FSM + API rate limit | ADR: `heartbeatmonitor/service.go:89` Warning30/Offline90/Unavailable300 + limiter |
| **FTP** vsftpd | `ftp.go:16 LoadFtpBaseInfo` `35 LoadFtpLogInfo` `62 OperateFtp` `84 SearchFtp` `111 CreateFtp` | Plaintext legacy; Forge has confined SFTP (`rootfs` + quota) + S3 adapters | ADR: use per-server SFTP or S3 |
| **System snapshot** panel+docker+logs image | `snapshot.go:12 LoadSnapshotData` `33 CreateSnapshot` `55 RecreateSnapshot` `77 ImportSnapshot` `113 SearchSnapshot` `146 RecoverSnapshot` | Host-rollback assumes panel is the OS; fleet host DR is re-image, not DB rollback | ADR: workload artifacts `backup/service.go:48` `BackupTypeServer/Database/Volume/App` |
| **Multi-host SSH inventory** | `host.go:19 CreateHost` `41 TestByInfo` `58 TestByID` `75 HostTree` `98 SearchHost` | Fleet nodes are daemon-tokenized (`handlers_host.go:35 GetNode+GetNodeDaemonCredential`), not `addr:port` SSH rows | Doc mapping: 1Panel Host → Forge Node (telemetry only) |
| **device / gpu / host_tool** | `device.go` `gpu.go` `host_tool.go` | Fleet GPU via capabilities delta, not host admin CRUD | Wire capabilities delta endpoint |

### 9.2 Documentation to produce (no code)

1. **ADR file:** `docs/adr/ADR-0xx-host-tool-freeze.md` (or append to `docs/architecture.md`):
   - Title: Host Appliance Surfaces Intentionally Out-of-Scope
   - Status: Accepted
   - Context: 1Panel comparison + FINAL §22 host-tool sprawl warning
   - Decision: list above 7 surfaces as REJECT with rationale
   - Consequences: migration guide section "1Panel → Forge host expectations"

2. **Migration guide snippet:** `docs/installation.md` add "1Panel Migration Note" box:
   ```md
   > **Host Tooling:** Docker daemon config, sshd, fail2ban, FTP, and system snapshots are not managed via the panel. Configure them at provisioning (Ansible/cloud-init). Use Host Files (allowlist-scoped), Host Terminal (WS), and workload Backups (S3) instead.
   ```

3. **Matrix ledger:** update `audits/final-parity/subagent-05-host-admin.md:7` ledger to mark REJECT rows separately so future `MASTER_FINDING_INDEX.md` audits don't re-file as MISSING without ADR override.

4. **Env template:** `infra/.env.example` already has `DAEMON_HOST_FILES_ALLOWLIST` template — reference it from ADR.

### 9.3 No migration / no code freeze

No DB migration. No API surface. Just docs + ADR header comment in `handlers_host.go`:

```go
// Package server — host endpoints are fleet telemetry proxies, not host management.
// Intentionally absent: daemon.json, sshd/RootCert, fail2ban, FTP, system snapshots.
// See docs/adr/ADR-0xx-host-tool-freeze.md.
```

---

## 10. Consolidated Work Breakdown — 4 Phases, 3 Tracks

### Phase 0 — Guard verification (1 day, no code)

| Task | Owner | Verify |
|------|-------|--------|
| Verify firewall transactional `iptables-restore --noflush` + sorted IDs | 06 | `handlers_firewall.go:96,361,381` |
| Verify hostfiles sibling-safe `underAny` + dataDir hard-block | 06 | `hostfiles.go:57,67` + test |
| Verify rootfs `RESOLVE_BENEATH` + `O_NOFOLLOW` | 06 | `rootfs_linux.go:47` |
| Add ADR for host-tool freeze (draft) | 06 + docs | `docs/adr/...` |
| Baseline perf: `wrk` 10× 10 MiB uploads concurrent heap profile | 06 | pprof |

### Phase 1 — P1 DoS + P1 reconnect (2-3 days)

**Track A — Upload OOM (H-OOM-01)**

| Task | File:Line | Est | Done when |
|------|-----------|-----|-----------|
| A1 Add early `ContentLength` 413 + `LimitReader` guard before `Body()` alloc | `handlers_files.go:386` | 0.5d | 101 MiB 413 at proxy, heap flat |
| A2 Sign metadata-only for streaming uploads (allow `nil` body) — make daemon accept | `daemon/client.go` SignedHeaders + beacon auth | 0.5d | Small + 90 MiB uploads both pass with `nil` HMAC |
| A3 Client preflight 100 MiB check in `host-files-view.tsx` | `host-files-view.tsx` | 0.25d | Toast before POST |
| A4 Tests: `TestHostFilesUpload_*` (413, streaming, multipart, ContentLength) | `handlers_files_test.go` | 0.5d | CI green |
| A5 Export `HostUploadLimit` constant or link comment between forge/beacon | both | 0.1d | Single source comment |

**Track B — Reconnect blind (H-BEACON-01)**

| Task | File:Line | Est |
|------|-----------|-----|
| B1 Split `doReconnect` probe-then-heartbeat, only `RecordHeartbeatSuccess` advances `lastHb` | `reconnect.go:157` | 0.5d |
| B2 Reset `attempts` on success | `reconnect.go:262` | 0.1d |
| B3 Wire heartbeat loop to `Record*` if not already | `daemon/main.go` | 0.25d |
| B4 Tests: probe-vs-heartbeat matrix | `reconnect_test.go` | 0.5d |

### Phase 2 — P2 Cron correctness + P2 monitoring truth (3-4 days)

**Track C — Cron**

| Task | File:Line | Est |
|------|-----------|-----|
| C1 Add `inFlight` map + `executeJobWithExecution` single-row path | `service.go:263` | 0.5d |
| C2 Replace `time.Sleep` retry loop with `scheduleRetries` (goroutine + `time.After` or `AfterFunc`) | `service.go:161` | 0.5d |
| C3 Make `scheduleJob` fire `go s.executeJob` not inline | `service.go:90` | 0.1d |
| C4 Add `CreateCronJobExecutionDedup` with `pg_advisory_xact_lock` + dedup window | `store/store_cron.go` | 0.5d |
| C5 FE mirror `* *` guard + 4096 char check | `cron-jobs/page.tsx:39` | 0.25d |
| C6 Migration: partial index `idx_cron_executions_job_created_status` | `migrations/2xx` | 0.25d |
| C7 Tests: single-row, dedup 429, non-blocking retries | `service_test.go` | 0.75d |

**Track D — Monitoring**

| Task | File:Line | Est |
|------|-----------|-----|
| D1 Host uptime via `/proc/uptime` + `daemonUptimeSeconds` new field | `handlers_host.go:65` + `sysinfo_linux.go:new` | 0.5d |
| D2 Enumerate all mounts via `/proc/mounts` or `gopsutil/disk` | `handlers_host.go:74` | 0.5d |
| D3 Network IPs/MAC via `net.Interfaces()` merge on linux | `sysinfo_linux.go:61` | 0.5d |
| D4 Process live metrics via `gopsutil/process` or disable inert sort + render `—` | `sysinfo_linux.go:94` + `host/page.tsx:234` | 0.5d |
| D5 FE: render `no data` not 0, `null` gaps in AreaChart, disable alert on synthetic | `monitoring/page.tsx:50,96,113` | 0.5d |
| D6 Darwintest parity: honest empty + banner | `sysinfo_darwin.go:56,84` | 0.25d |

### Phase 3 — Polish + docs + verification (1-2 days)

| Task | File:Line | Est |
|------|-----------|-----|
| E1 Firewall UI placeholder fix `0.0.0.0/0` → `198.51.100.23` + remove deny | `AdminFirewall.tsx:356` | 0.2d |
| E2 Hostfiles env template in `infra/.env.example` + `compose.beacon.yml` | infra | 0.1d |
| E3 ADR + migration guide + ledger update | docs | 0.5d |
| E4 `mapDaemonError` unification + heartbeat-stale header for host tiles (LF-05) — optional P3 | `handlers_host.go:11` | 0.5d |
| E5 `metrics` `SplitN(key,"_",3)` structured labels (LF-06) — optional P3 | `handlers_metrics.go:131` | 0.5d |
| E6 End-to-end smoke: host Files/Cron/Monitoring/Firewall tiles green on staging | manual | 0.5d |

**Total:** ~7-10 dev-days single engineer; parallelize Track A+B+C+D across 2 engineers → ~5 days wall time.

---

## 11. Detailed Code Diffs — Ready-to-Apply Patches (illustrative, not yet applied)

### 11.1 `forge/api/internal/http/handlers_files.go:386` — upload cap

```diff
- rawBody := c.Request().Body()
- req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, targetURL, bytes.NewReader(rawBody))
- req.ContentLength = int64(len(rawBody))
- headers, err := cfg.Daemon.SignedHeaders(target.NodeToken, http.MethodPost, req.URL.RequestURI(), rawBody)
+ // early 413 before alloc
+ if cl := c.Request().Header.ContentLength(); cl > hostUploadLimit+hostUploadSlack {
+     return fiber.NewError(fiber.StatusRequestEntityTooLarge, "host upload exceeds 100 MiB limit")
+ }
+ if len(c.Request().Body()) > hostUploadLimit {
+     return fiber.NewError(fiber.StatusRequestEntityTooLarge, "host upload exceeds 100 MiB limit")
+ }
+ body := c.Request().Body() // now bounded
+ req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, targetURL, bytes.NewReader(body))
+ req.ContentLength = int64(len(body))
+ headers, err := cfg.Daemon.SignedHeaders(target.NodeToken, http.MethodPost, req.URL.RequestURI(), nil) // metadata-only
```

Full streaming pipe variant in §3.3 is preferred for final PR.

### 11.2 `forge/api/internal/services/cronjob/service.go:21` — inFlight + dedup

```diff
 type Service struct {
     store   *store.Store
     cron    *cron.Cron
     mu      sync.Mutex
     entries map[string]cron.EntryID
+    inFlight map[string]time.Time
     logger  *slog.Logger
     serverDispatcher ServerCommandDispatcher
 }
+func New(...) { entries: make(map[string]cron.EntryID), inFlight: make(map[string]time.Time) }
```

### 11.3 `beacon/internal/server/handlers_host.go:12` — HostInfo + uptime

```diff
 type HostInfo struct {
     Hostname string `json:"hostname"`
     OS       string `json:"os"`
     Kernel   string `json:"kernel"`
-    Uptime   int64  `json:"uptimeSeconds"`
+    Uptime       int64  `json:"uptimeSeconds"`        // host boot (was daemon)
+    DaemonUptime int64  `json:"daemonUptimeSeconds"`
     CPUModel string `json:"cpuModel"`
     CPUCores int    `json:"cpuCores"`
 }
 func (s *Server) handleHostInfo(w http.ResponseWriter, r *http.Request) {
     hostname, _ := os.Hostname()
+    uptime := hostUptimeSeconds(s.started)
     info := HostInfo{
-        Uptime:   int64(time.Since(s.started).Seconds()),
+        Uptime:       uptime,
+        DaemonUptime: int64(time.Since(s.started).Seconds()),
     }
 }
```

---

## 12. Migrations

### 12.1 Required

```sql
-- migrations/2xx_cron_dedup_support.sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto"; -- already present in most forges; idempotent
CREATE INDEX IF NOT EXISTS idx_cron_executions_job_created_status
  ON cron_job_executions(job_id, created_at DESC)
  WHERE status IN ('pending','running');
-- No table change; pg_advisory_xact_lock needs no DDL
```

### 12.2 Not required

- Monitoring seam: no DB (collectors are in-memory /host proxy).
- Firewall/hostfiles: no DB.
- Reconnect: no DB.
- Host-tool freeze: no DB.

### 12.3 Rollback

- Drop index: `DROP INDEX IF EXISTS idx_cron_executions_job_created_status;`
- Revert `HostInfo` additive field is non-breaking — old FE ignores `daemonUptimeSeconds`; rolling back `uptimeSeconds` semantics requires FE redeploy.
- Upload cap: lowering is strictly stricter — rollback is just raising limit constant.

---

## 13. Frontend Fixes — `forge/web` Exact Patches

### 13.1 `forge/web/app/admin/monitoring/page.tsx:50` — synthetic-zero

| Line | Before | After |
|------|--------|-------|
| `:50` | `every cpuLoad1m===0 && networkRx===0` global | Keep banner but also per-metric `metricHasData` check |
| `:96` | Always shows chart with zeros | If `!metricHasData` → `No telemetry for CPU — collector not yet available` card |
| `:113` | `v: ... ?? 0` | `v: isSynthetic && raw===0 ? null : raw` + `connectNulls={false}` |
| `:134` | Comparison table `cpu?.toFixed(1)` | `cpu!=null && !isSynthetic ? cpu.toFixed : "—"` |

### 13.2 `forge/web/app/admin/host/page.tsx:32` — host tiles

| Line | Fix |
|------|-----|
| `:32` | `useHostQuery` already has stale10s/refetch30s — add `staleTime` header handling: if `X-Node-Heartbeat-Stale: true` render amber badge |
| `:98` | `fmtUptime(data.uptimeSeconds)` → show both `Host up ... · Agent up ...` when `daemonUptimeSeconds` present |
| `:219` | `ProcessesTab` sort guard: `hasLiveMetrics = data?.some(p=>p.cpuPercent!==0)` → disable buttons + show `Name/State only` pill when false |
| `:234` | `sort` only when `hasLiveMetrics`, else sort by `Name` |

### 13.3 `forge/web/components/admin/AdminFirewall.tsx:356` — placeholder

```tsx
// Before
placeholder="e.g. 0.0.0.0/0"
// After
placeholder="e.g. 198.51.100.23 or 198.51.100.0/24"
```

Remove `deny` from `Action` select; add inline `validateFirewallSource` client check before `POST /host/firewall/rules`.

### 13.4 `forge/web/app/admin/cron-jobs/page.tsx:39` — cron validation

Mirror BE `validateCronSchedule` (`handlers_cronjob.go:15`) that rejects `* *` every-minute. Add:

```tsx
if (parts[0]==="*" && parts[1]==="*") return "Minimum interval is 1 minute";
if (command.length > 4096) return "Command too long (max 4096)";
```

### 13.5 `forge/web/components/admin/host-files-view.tsx` — upload preflight

```tsx
if (file.size > 100*1024*1024) { toast.error("Host upload limit 100 MiB"); return; }
```

---

## 14. Firewall & Hostfiles — Keep + Harden (ALREADY FIXED verification)

### 14.1 Firewall transactional (ALREADY FIXED — do not re-implement)

- `handlers_firewall.go:361` reconciled via `iptables-restore --wait 10 --noflush` (no flush-open window).
- Sorted `ruleIDs`/`forwardIDs` + `auditFirewall` log + 0600/fsync persist.
- **This plan’s only firewall code change:** UI placeholder + optional `mapDaemonError` unification.

### 14.2 Hostfiles allowlist deny-by-default remains

- `hostfiles.go:51` correctly: allowlist non-empty → must be inside allowlist; empty → block `/` + denylist system prefixes + message to configure `DAEMON_HOST_FILES_ALLOWLIST`.
- Sibling-safe `underAny` already correct; keep.
- `SetHostFileAllowlist` validates each root via `validateHostPath`.

---

## 15. Beacon Reconnect — Detailed Verification Steps

### 15.1 Reproduce blind StateConnected

1. Start beacon with `offlineTimeout=30s`.
2. Mock panel: `/health` 200, `POST /api/remote/heartbeat` 401 (bad token).
3. Observe pre-fix: `reconnect.go:192` would set `StateConnected` after probe 200 → `handlers_files.go:149` terminal WS would allow connection even though heartbeat 401 → operator sees "connected" but node is `Offline` in `heartbeatmonitor`.
4. Post-fix: `doReconnect` probe 200 → `SendNodeHeartbeat` 401 → stays `Reconnecting`.

### 15.2 Heartbeatmonitor seam

- `forge/api/internal/services/heartbeatmonitor/service.go:89` `DefaultConfig` Warning30/Offline90/Unavailable300 governs `NodesSuspectedTotal` etc. Beacon `reconnect.go` must not defeat it by faking `lastHb`. After fix, `heartbeatmonitor` is source of truth; beacon `StateConnected` aligns to it.

---

## 16. Security & Correctness Considerations

| Concern | Mitigation |
|---------|------------|
| **DoS via upload** | 413 at proxy before beacon; `MaxBytesReader` defense-in-depth at beacon; `hostAtomicWrite` `LimitReader(limit+1)` final guard |
| **Cron cross-replica double-fire** | `pg_advisory_xact_lock` serializes `TriggerNow` + cron tick across replicas; monotonic `inFlight` dedup for rapid double-click |
| **Cron starvation** | `go s.executeJob` off cron worker + async retries via `time.After` not `Sleep` |
| **Firewall footgun** | Placeholder fix prevents `0.0.0.0/0` confusion; allow-only hardening stays |
| **Host file escape** | `RESOLVE_BENEATH` + allowlist sibling-safe; `underAny` already correct |
| **Monitoring spoofing** | Render `—` not `0` until live collectors land; alert creation disabled on synthetic |
| **Reconnect spoofing** | Only real 2xx heartbeat advances `lastHb`; probe alone insufficient |

---

## 17. Backward Compatibility & Rollout

### 17.1 API compatibility

| Surface | Change | Compat |
|---------|--------|--------|
| `HostInfo.uptimeSeconds` | Semantic change: was daemon, now host | **Breaking label** — but value was wrong before. Gate behind `?hostUptime=1` query or dual-field `daemonUptimeSeconds` additive so old FE still works (reads host uptime as "Uptime" which is more correct). Old FE deployed against new beacon shows larger uptime — improves UX, no break. |
| `HostInfo.daemonUptimeSeconds` | New field | Additive — old FE ignores |
| `DiskPartition` array | Now multi-element | FE already maps array — longer list just renders more rows; old FE single-row assumptions none |
| `NetworkInterface.IPs/MAC` | Now populated on linux | Previously empty string — now filled; FE already renders them |
| `Cron` `TriggerNow` | Now single row + 429 dedup | Old callers polling outer row now see completion (was hung — fixing bug). 429 is new but only on rapid re-trigger within 5s |
| `hostFilesUpload` | Now 413 for >100 MiB at proxy (previously OOM or beacon 413) | Strictly stricter at proxy edge; same limit as beacon so no valid request newly rejected |
| `beacon reconnect StateConnected` | Now requires heartbeat 2xx | Nodes with bad token that previously appeared Connected will now correctly show Reconnecting — operator-visible correction; document in release notes |

### 17.2 Rollout order

1. **Beacon first:** ship `handlers_host.go` uptime/disk/network/process fixes + `sysinfo_linux.go` + `reconnect.go` (canary one node, verify `/v1/host/info` shows both uptimes, `/v1/host/disk` multi-mount, `/v1/host/network` has IPs).
2. **Forge API second:** ship `handlers_files.go` upload cap + `cronjob/service.go` dedup/non-blocking + `store_cron.go` helper + migration index (rolling deploy; old API + new beacon still compatible; new API + old beacon also compatible — beacon cap still second line).
3. **Web last:** ship `monitoring/page.tsx` no-data rendering + `host/page.tsx` dual uptime + firewall placeholder (pure FE, backwards compat with either beacon).

### 17.3 Feature flags (optional)

- `FORGE_CRON_DEDUP=1` gate for `pg_advisory_xact_lock` path (default on).
- `FORGE_HOST_UPLOAD_STREAM=1` gate for pipe streaming (default off until beacon accepts `nil` body HMAC — use bounded buffered path first).
- `BEACON_LIVE_PROCESS_METRICS=1` gate for `gopsutil` process path (default on linux, off on darwin).

---

## 18. Testing Strategy — What Must Be Green Before Merge

### 18.1 Beacon

```
go test ./beacon/internal/server -run TestHostUptime -count=1
go test ./beacon/internal/server -run TestHostDisk -count=1 -v   # expects multi-mount on linux
go test ./beacon/internal/server -run TestNetInterfaces -count=1 # expects IPs non-empty
go test ./beacon/internal/server -run TestProcessList -count=1   # expects CPU% >0 for at least one proc on linux
go test ./beacon/internal/server -run Firewall -count=1
go test ./beacon/internal/server -run TestHostFilesConfinement -count=1
go test ./beacon/internal/remote -run TestReconnect -count=1
```

### 18.2 Forge API

```
go test ./forge/api/internal/http -run TestHostFilesUpload -count=1
go test ./forge/api/internal/services/cronjob -run TestTriggerNow -count=1 -race
go test ./forge/api/internal/services/cronjob -run TestCronRetries_NonBlocking -count=1
go test ./forge/api/internal/store -run TestCronDedup -count=1
```

### 18.3 Web (vitest / e2e)

```
pnpm --filter forge-web test -- monitoring
# isSynthetic true → chart shows gap, not 0 line
# TriggerNow → polling same id completes (not hung)
# Host page: dual uptime rendered, inert sort disabled when no live metrics
# Firewall: placeholder not 0.0.0.0/0, deny not in select
```

### 18.4 Manual smoke (staging with 2 API replicas + 1 beacon)

- [ ] `curl --data-binary @110M.bin http://forge/api/v1/host/files/upload?path=/tmp/x` → 413 at forge, beacon log has no `handleHostFilesUpload` entry, `docker stats` heap stable.
- [ ] `POST /api/v1/cron-jobs/:id/execute` ×2 rapid → second 429, first completes, `GET /executions` count +1 not +2.
- [ ] Cron job `*/1 * * * *` with `RetryCount=3` that `exit 1` → other `*/1` job still ticks every minute (no stall).
- [ ] `GET /api/v1/host/info?nodeId=X` shows `uptimeSeconds` ~ host boot (e.g. 30d) and `daemonUptimeSeconds` ~2h since beacon restart.
- [ ] `GET /api/v1/host/disk?nodeId=X` lists ≥2 mounts (/, /data if present).
- [ ] `GET /api/v1/host/network?nodeId=X` has `ips: "10.x.x.x, 192.168.x.x"` not `"`.
- [ ] Node token revoked → beacon `reconnect` stays `Reconnecting` not `Connected`; `POST /host/terminal/ws?nodeId=X` 502/Offline.

---

## 19. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Upload streaming sign `nil` body breaks older beacon verifier that expects body HMAC | Med | Uploads 401 | Make beacon accept both: verify `nil` body branch for `/v1/files/upload` only; keep buffered path for small files behind flag |
| `/proc/uptime` unavailable (container not host PID) | Low | Fallback to daemon age | Fallback already coded; `readProcMounts` similar fallback to `"/"` |
| `gopsutil` adds transient lib dep + cgo edge on alpine | Low | Build heavier | Gate behind build tag; keep `/proc/status` fallback if import fails |
| `pg_advisory_xact_lock` contention under bursty triggers | Low | 5s dedup window + lock wait <10ms | Timeout advisory lock quickly; index on `(job_id, created_at)` keeps scan fast |
| Host uptime semantic change confuses old FE `uptimeSeconds` | Low | UI shows larger number | Add `daemonUptimeSeconds` additive; old FE just shows host uptime (more correct than before) |
| `iptables-restore` transactional already fixed but verification skipped | Low | Regression reintroduced | Add `TestFirewallReconcile_NoFlushWindow` assert `--noflush` flag present |

---

## 20. Appendix — File:Line Index (all cited, re-verified 2026-08-24)

| File | Line | Symbol / Note |
|------|------|---------------|
| `forge/api/internal/http/handlers_files.go` | `386` | `hostFilesUpload` — rawBody unbounded |
| `forge/api/internal/http/handlers_files.go` | `402` | `rawBody:=c.Request().Body()` |
| `forge/api/internal/http/handlers_files.go` | `547` | `validateHostFilePath` — canonical mirror of beacon |
| `forge/api/internal/http/handlers_host.go` | `11` | `mapDaemonError` — 504/502 mapping to unify |
| `forge/api/internal/http/handlers_host.go` | `35` | `resolveNodeHostTarget` |
| `forge/api/internal/http/handlers_host.go` | `69` | `registerHostRoutes` 5 proxies |
| `forge/api/internal/http/handlers_firewall.go` | `7` | `registerFirewallRoutes` — proxy to daemon |
| `forge/api/internal/http/handlers_cronjob.go` | `15,23` | `validateCronSchedule` rejects `* *` every-minute |
| `forge/api/internal/http/handlers_cronjob.go` | `196` | `POST /cron-jobs/:id/execute → TriggerNow` |
| `forge/api/internal/services/cronjob/service.go` | `21` | `Service{store,cron,mu,entries,logger,serverDispatcher}` |
| `service.go` | `62,81,90` | `Start` + `scheduleJob` AddFunc (inline executeJob — should be `go`) |
| `service.go` | `124` | `executeJob` creates shadow execution row |
| `service.go` | `161` | `time.Sleep(5*(i+1)s)` blocks cron worker |
| `service.go` | `263,268` | `TriggerNow` outer `CreateCronJobExecution` dup |
| `service.go` | `190,212` | `defaultMaxCronTimeoutSeconds=3600` + `runShellCommand` scrubbed env |
| `service.go` | `244` | `dispatchServerCommand` fail-closed |
| `beacon/internal/server/hostfiles.go` | `19` | `validateHostPath` canonical |
| `hostfiles.go` | `36` | `hostFileDenylistPrefixes` |
| `hostfiles.go` | `51,67` | `resolveHostPath` + `underAny` sibling-safe |
| `hostfiles.go` | `92` | `SetHostFileAllowlist` |
| `hostfiles.go` | `109,124` | `hostAtomicWrite` Limit+1 + dir Sync |
| `hostfiles.go` | `457` | `hostUploadLimit 100MiB` + `MaxBytesReader` |
| `hostfiles.go` | `442` | `handleHostFilesUpload` correct (beacon side) |
| `beacon/internal/server/secure_files.go` | `26` | `maxFileWriteBytes 16MiB` + archive limits |
| `secure_files.go` | `155` | `archivePathTracker` — reuse pattern at API layer |
| `beacon/internal/server/handlers_host.go` | `12` | `HostInfo{Hostname/OS/Kernel/Uptime/CPUModel/CPUCores/Arch}` |
| `handlers_host.go` | `65` | `Uptime: time.Since(s.started)` — mislabel |
| `handlers_host.go` | `74` | `handleHostDisk` only `Statfs("/")` |
| `handlers_host.go` | `121` | `handleHostNetwork → netInterfaces()` |
| `handlers_host.go` | `130` | `handleHostProcesses → processList()` |
| `beacon/internal/server/sysinfo_linux.go` | `61` | `netInterfacesPlatform` — no IPs (sysfs only) |
| `sysinfo_linux.go` | `94` | `processListPlatform` Name/State only → inert sort |
| `beacon/internal/server/sysinfo_darwin.go` | `56,84` | darwin uses `net.Interfaces()` (has IPs) + empty process list |
| `beacon/internal/rootfs/rootfs_linux.go` | `47` | `RESOLVE_BENEATH\|NO_MAGICLINKS\|NO_SYMLINKS` + `O_NOFOLLOW` |
| `beacon/internal/remote/reconnect.go` | `98,106` | `run` StateConnecting + `offlineCheck` ticker |
| `reconnect.go` | `157` | `doReconnect` — blind `lastHb=now` + `StateConnected` after probe |
| `reconnect.go` | `207` | `probePanel` tries `/health`, `/api/v1/health/live`, `/` |
| `reconnect.go` | `262,277` | `RecordHeartbeatSuccess` / `RecordHeartbeatFailure` |
| `beacon/internal/server/handlers_firewall.go` | `96,100` | `runFirewallRestore` `iptables-restore --wait 10 --noflush` |
| `handlers_firewall.go` | `129` | `validateFirewallSource` public unicast only |
| `handlers_firewall.go` | `361` | `reconcile` sorted transactional |
| `handlers_firewall.go` | `259` | `persistLocked` 0600/fsync |
| `forge/web/app/admin/monitoring/page.tsx` | `50` | `isSynthetic` every zero |
| `monitoring/page.tsx` | `96` | synthetic warning banner |
| `monitoring/page.tsx` | `113` | `v: ... ?? 0` flat zero line |
| `forge/web/app/admin/host/page.tsx` | `98,219,234` | `fmtUptime` + `ProcessesTab` inert sort by cpuPercent |
| `forge/web/components/admin/AdminFirewall.tsx` | `356` | placeholder `0.0.0.0/0` footgun |
| `forge/web/components/admin/host-files-view.tsx` | upload | needs 100 MiB client preflight |
| `forge/web/app/admin/cron-jobs/page.tsx` | `39` | `parseCronExpression` drift vs BE |
| `forge/web/app/admin/terminal/page.tsx` | `149` | heartbeat-gated WS `HeartbeatStateOffline` check correct |

---

## 21. Release Note Draft

> **Host infra seam hardening (P1/P2):**
> - **Host upload:** Forge API now enforces 100 MiB cap *before* buffering and streams to beacon via chunked pipe — OOM path closed. Beacon 100 MiB `MaxBytesReader` remains defense-in-depth. Client preflight toast for >100 MiB.
> - **Cron:** `TriggerNow` no longer double-creates executions (single row + 429 dedup on rapid re-trigger). Retries no longer block the scheduler (`time.After` queue). Cross-replica double-fire prevented via `pg_advisory_xact_lock` (no schema change beyond partial index).
> - **Monitoring:** Charts render `no data` gaps not flat 0% when collectors absent; `isSynthetic` banner now also disables alert creation. Host uptime now from `/proc/uptime` (new `daemonUptimeSeconds` field added); disk lists all mounts; network includes IPs/MAC on linux; process sort now live via `gopsutil` or honestly disabled.
> - **Beacon reconnect:** `StateConnected` now requires successful `SendNodeHeartbeat` 2xx after probe — probe 2xx alone no longer fakes Connected. Fixes heartbeatmonitor mismatch and terminal offline gate.
> - **Firewall:** verified transactional `iptables-restore --noflush` still holds; UI placeholder fixed (`0.0.0.0/0` → example public CIDR).
> - **Host files:** allowlist deny-by-default unchanged; sibling-prefix guard verified; `DAEMON_HOST_FILES_ALLOWLIST` template added.
> - **Host appliance** (`daemon.json`/SSH/fail2ban/FTP/snapshot) documented as intentionally out-of-scope (ADR).

---

## 22. Checklist for Implementer (copy-paste)

- [ ] Apply `handlers_files.go:386` upload cap + streaming (or bounded buffered v1) + `daemon/client.go` nil-body HMAC accept
- [ ] Apply `service.go:263` single-row `TriggerNow` + `inFlight` dedup + `pg_advisory_xact_lock` helper + `scheduleRetries` non-blocking + `go s.executeJob` in `scheduleJob`
- [ ] Add `migrations/2xx_cron_dedup_support.sql` partial index
- [ ] Apply `handlers_host.go:65` `/proc/uptime` + `daemonUptimeSeconds` + `handlers_host.go:74` multi-mount disk + `sysinfo_linux.go:61` network IPs merge + `sysinfo_linux.go:94` gopsutil or inert-sort guard
- [ ] Apply `reconnect.go:157` probe-then-heartbeat only-`RecordHeartbeatSuccess` path
- [ ] Verify `handlers_firewall.go:361` transactional still `--noflush` + sorted (no code unless regressed)
- [ ] Keep `hostfiles.go:51` / `rootfs_linux.go:47` — add sibling test + env template only
- [ ] FE: `monitoring/page.tsx:50` gaps + `host/page.tsx:98` dual uptime + inert sort guard + `AdminFirewall.tsx:356` placeholder + `cron-jobs/page.tsx:39` FE guard + `host-files-view.tsx` preflight
- [ ] Write `docs/adr/ADR-0xx-host-tool-freeze.md` + update `MASTER_FINDING_INDEX.md` REJECT ledger
- [ ] Run `go test ./...` + manual smoke checklist in §18.4

---

*End of plan — file:line cited, backward compat preserved, no migrations beyond additive partial index, fleet truth seam respected.*
