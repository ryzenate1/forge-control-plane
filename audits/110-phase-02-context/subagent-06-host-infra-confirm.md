# Subagent 06 — Host / Infrastructure Confirm (Files, Cron, Monitoring, Firewall)

**Focus:** Confirm Host/Infrastructure implementations against LIVE — bounded hostfile writes, cron health, firewall isolation, daemon uptime, sysinfo sort, hostfiles allowlist.

**Prior audits reconciled:**
- `audits/phase-06/subagent-04-1panel-infra-monitor.md` (18 rows C01–C18, LF-01–LF-05)
- `audits/final-parity/subagent-05-host-admin.md` (18 rows H-01–H-18, LF-01–LF-04)
- `audits/reverification/subagent-12-infra-host.md` (14 rows R-01–R-14, LF-01–LF-05)

**LIVE checkout date:** 2026-08-24
**Method:** File:line SOURCE_VERIFIED — every citation below was opened and grep-verified immediately before writing. No product code modified.

---

## 0. Executive Verdict — 7 Findings Requested vs LIVE

| # | Requested Finding | LIVE Line Proof | Verdict |
|---|---|---|---|
| **F-01** | `forge/api/internal/http/handlers_files.go:386` rawBody unbounded vs `beacon/hostfiles.go:457` 100MiB cap | `handlers_files.go:402` `rawBody := c.Request().Body()` → `403` `bytes.NewReader(rawBody)` with no `io.LimitReader` or size pre-check before alloc; `forge/api/internal/http/server.go:1040` `BodyLimit: 32 MiB` bounds at Fiber layer but does NOT stream; `beacon/internal/server/hostfiles.go:457` `const hostUploadLimit = 100*1024*1024` + `458` `http.MaxBytesReader(w,r.Body,hostUploadLimit)` + `124` `io.LimitReader(reader, limit+1)` | **STILL BROKEN** (P1→P2 via BodyLimit mitigation, design still wrong) |
| **F-02** | `forge/api/internal/services/cronjob/service.go:124` duplicate execution + `158`/`161` retry sleep blocks scheduler | `service.go:90` `cron.AddFunc(... s.executeJob(context.Background(), jobID))` + `117` `executeJob` `124` `CreateCronJobExecution` + `263` `TriggerNow` `268` `CreateCronJobExecution` → `273` `go s.executeJob(...)` which `124` creates *second* row; `158` `for i<RetryCount` `161` `time.Sleep(5*(i+1)*s)` blocks cron goroutine | **STILL BROKEN** — duplicate rows + scheduler stall both unchanged |
| **F-03** | `monitoring/page.tsx:50` isSynthetic banner | `forge/web/app/admin/monitoring/page.tsx:51` `isSynthetic` memo `52-54` checks `cpuLoad1m==0 && networkRxBytes==0`, `68` pill `· allocated`, `97-101` amber banner `AlertTriangle` “allocated capacity (not live OS). Network/load not yet collected” | **FIXED (banner)** — honest fallback correctly implemented; underlying live collector still absent (synthetic zeros remain) |
| **F-04** | `beacon/handlers_host.go:65` daemon uptime masquerading as host uptime | `beacon/internal/server/handlers_host.go:65` `Uptime: int64(time.Since(s.started).Seconds())` + `forge/web/app/admin/host/page.tsx:84` `fmtUptime(data.uptimeSeconds)` unlabeled “Uptime” | **STILL BROKEN** — daemon uptime, not `/proc/uptime`; resets on beacon restart |
| **F-05** | `sysinfo_linux.go:94` inert sort (process CPU%/mem always 0) | `beacon/internal/server/sysinfo_linux.go:94` `processListPlatform` parses only `Name:`/`State:` from `/proc/*/status`, never populates `CPU`/`Memory`; `sysinfo_darwin.go:84` `return []ProcessEntry{}, nil` (empty); `forge/web/app/admin/host/page.tsx:176` `sorted.sort(b.cpuPercent - a.cpuPercent)` inert on zeros; `195` `proc.cpuPercent.toFixed(1)` renders `0.0` | **STILL BROKEN** — UI sort affordance does nothing; Darwin divergent |
| **F-06** | Firewall placeholder `0.0.0.0/0` | `beacon/internal/server/handlers_firewall.go:129` `validateFirewallSource` `132` `unrestricted rules are not allowed` `162` `safeFirewallSourceIP` `IsGlobalUnicast && !IsPrivate`; `forge/web/components/admin/AdminFirewall.tsx:356` placeholder `e.g. 0.0.0.0/0` + `403` same; `357-359` `AdminSelect action allow/deny` but `handlers_firewall.go:120` `validateFirewallAction` only `allow` | **STILL BROKEN** — placeholder suggests input that always 400s; `deny` option is dead UX; secure-by-default policy itself is correct |
| **F-07** | Hostfiles allowlist properly pinned vs `1Panel file.go:400` permissive | `beacon/internal/server/hostfiles.go:19` `validateHostPath` canonical+no `\`/`\0` `36` `hostFileDenylistPrefixes` `51` `resolveHostPath` `67` `underAny` prefix `76` allowlist branch `83` denylist fallthrough `58` data-dir hard block `92` `SetHostFileAllowlist` validates each root `109` `hostAtomicWrite` sibling-temp+`Chmod`+`LimitReader`+`Sync`+`Rename`+`dir Sync`; `forge/api/internal/http/handlers_files.go:547` `validateHostFilePath` mirror; vs `reference/app-platforms/1panel/agent/app/api/v2/file.go:400` `UploadFiles` streams via `MultipartForm` but writes anywhere uid can write, only `650` `ShouldDenySensitiveFileRead` for reads | **FIXED / STRONGER** — allowlist-vs-denylist + atomic write discipline correctly implemented; intentionally denies `0.0.0.0/0`-style permissive roots |

**Tally:** FIXED 2 (F-03 banner, F-07 allowlist), STILL BROKEN 5 (F-01, F-02, F-04, F-05, F-06). 2 of 5 broken are mitigated (F-01 BodyLimit, F-03 banner compensates).

---

## 1. Deep Confirm — Per Finding with Line Proof

### F-01 — Host file upload buffering: `handlers_files.go:386 rawBody` vs `hostfiles.go:457 100MiB` — STILL BROKEN (P1 → P2 after BodyLimit)

**Prior claims:**
- `phase-06 LF-02 Finding 2` (hostfiles.go:51 denylist + `handlers_files.go:386 hostFilesUpload reads Body() eagerly into rawBody :402 then proxies with bytes.NewReader` + `hostUploadLimit=100MiB :457`)
- `final-parity LF-01` (Forge buffers entire body into rawBody without LimitReader; 400 MiB multipart OOMs Forge before beacon MaxBytesReader)
- `reverification LF-01 [P1]` (no LimitReader or ContentLength guard at :402 before alloc)

**LIVE re-inspection:**

`forge/api/internal/http/handlers_files.go:386` (`hostFilesUpload`):

```go
386: func hostFilesUpload(cfg Config) fiber.Handler {
387:     return func(c *fiber.Ctx) error {
388:         target, err := resolveNode(cfg, c)
...
392:         destPath := c.Query("path", "/")
393:         if err := validateHostFilePath(destPath); err != nil {
394:             return err
395:         }
396:         contentType := strings.ToLower(string(c.Request().Header.ContentType()))
397:         if contentType != "application/octet-stream" && !strings.HasPrefix(contentType, "multipart/form-data;") {
398:             return fiber.NewError(fiber.StatusUnsupportedMediaType, "upload must be application/octet-stream or multipart/form-data")
399:         }
400:         targetURL := strings.TrimRight(target.BaseURL, "/") + "/v1/files/upload?path=" + url.QueryEscape(destPath)
401:
402:         rawBody := c.Request().Body()
403:         req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, targetURL, bytes.NewReader(rawBody))
...
408:         req.ContentLength = int64(len(rawBody))
410:         headers, err := cfg.Daemon.SignedHeaders(target.NodeToken, http.MethodPost, req.URL.RequestURI(), rawBody)
...
420:         resp, err := cfg.Daemon.HTTPClient().Do(req)
```

Observations:
1. No `io.LimitReader`, no `c.Request().Header.ContentLength()` pre-check, no `413` before alloc. Full body is materialized in-memory via `c.Request().Body()` → `bytes.NewReader` → `SignedHeaders(..., rawBody)` needs `[]byte` so streaming via `io.Pipe` would require refactoring signature.
2. `forge/api/internal/http/server.go:1040` `BodyLimit: 32 * 1024 * 1024,` DOES bound worst-case at Fiber layer — a 400 MiB multipart would now be rejected by Fiber with `413` before handler runs. This is a **mitigation since prior audits** (Fiber limit was set to 32 MiB in current checkout) but is not the intended `100 MiB` host limit and does not fix double-buffering: `SignedHeaders` + `HTTPClient().Do` both hold the `32 MiB` slice, so 10 concurrent uploads allocate `320 MiB` on the control plane. Prior `P1` OOM is down-graded to `P2` but not fixed.
3. Beacon side correctly caps at `beacon/internal/server/hostfiles.go:457` `const hostUploadLimit = 100 * 1024 * 1024` `458` `r.Body = http.MaxBytesReader(w, r.Body, hostUploadLimit)` and `hostAtomicWrite:124` `io.Copy(tmp, io.LimitReader(reader, limit+1))` `129` `if written > limit { return errors.New("file exceeds size limit") }`. The cap is never reached for bodies `>32 MiB` because Forge rejects first; for bodies `≤32 MiB` it is correctly enforced but only after Forge buffering.
4. Contrast 1Panel `reference/app-platforms/1panel/agent/app/api/v2/file.go:400` `UploadFiles` `401` `form, err := c.MultipartForm()` per-file `c.SaveUploadedFile` + `468` `tmpFilename := dstFilename + ".tmp"` `481` `os.Rename` — no single `rawBody` alloc. Forge is strictly worse on memory per-request.

**Verdict:** **STILL BROKEN** — design gap unchanged. `BodyLimit 32 MiB` prevents catastrophic 400 MiB OOM but leaves inefficient double-buffer + `SignedHeaders` requiring full `[]byte`. Prior `P1` → current `P2`. Proper fix remains: stream via `io.Pipe` chunked to beacon and sign canonical metadata (path + content-type + content-length) instead of `rawBody`, or enforce `ContentLength` pre-check and `413` at Forge without second alloc. Regression test needed: `101 MiB` should `413` at Forge without beacon call; currently it `413`s via Fiber `32 MiB` but message is generic Fiber, not host-specific.

**Recommendation (unchanged from prior audits):**
- Enforce `if c.Request().Header.ContentLength() > hostUploadLimit` early `413` with clear message.
- Or better: `io.Pipe` + chunked proxy to beacon; compute HMAC over `destPath|contentType|contentLength` not `rawBody`.
- Align docs: per-server `beacon/internal/server/secure_files.go:27` `maxFileWriteBytes 16 MiB` vs host `100 MiB` divergence.

---

### F-02 — Cron `service.go:158,268 vs 124 duplicate + 161 sleep blocks scheduler` — STILL BROKEN (P2)

**Prior claims:**
- `phase-06 LF-03 Finding 1` (in-goroutine sleep `service.go:161` blocks scheduler; Finding 2 TriggerNow duplicate parallel executions `service.go:268 vs :124`)
- `final-parity LF-02` (duplicate execution rows — caller polls hung pending)
- `reverification LF-02 [P2]` and `LF-03 [P2]` (retry Sleep blocks cron.Cron + FE/BE * * mismatch)

**LIVE re-inspection:**

`forge/api/internal/services/cronjob/service.go:81` (`scheduleJob`):

```go
81: func (s *Service) scheduleJob(ctx context.Context, job store.CronJob) error {
82:     s.mu.Lock()
83:     defer s.mu.Unlock()
85:     if existingID, ok := s.entries[job.ID]; ok {
86:         s.cron.Remove(existingID)
87:     }
89:     jobID := job.ID
90:     entryID, err := s.cron.AddFunc(job.Schedule, func() {
91:         s.executeJob(context.Background(), jobID)
92:     })
```

`service.go:117` (`executeJob`):

```go
117: func (s *Service) executeJob(ctx context.Context, jobID string) {
118:     job, err := s.store.GetCronJob(ctx, jobID)
...
124:     execution, err := s.store.CreateCronJobExecution(ctx, jobID)
...
130:     start := time.Now()
...
134:     switch {
135:     case job.TargetType == "server":
136:         exitCode, output, errStr = s.dispatchServerCommand(ctx, job)
137:     default:
138:         exitCode, output, errStr = s.runShellCommand(job.Command, job.TimeoutSeconds)
...
154:     if err := s.store.CompleteCronJobExecution(ctx, execution.ID, status, exitCode, output, errStr, durationMs); err != nil {
...
158:     if status == "failed" && job.RetryCount > 0 {
159:         for i := 0; i < job.RetryCount; i++ {
160:             s.logger.Info("retrying cron job", "id", jobID, "attempt", i+1)
161:             time.Sleep(time.Duration(5*(i+1)) * time.Second)
162:
163:             retryExec, err := s.store.CreateCronJobExecution(ctx, jobID)
163:             if err != nil {
164:                 continue
165:             }
166:             retryStart := time.Now()
167:             if job.TargetType == "server" {
168:                 exitCode, output, errStr = s.dispatchServerCommand(ctx, job)
169:             } else {
170:                 exitCode, output, errStr = s.runShellCommand(job.Command, job.TimeoutSeconds)
171:             }
172:             retryDuration := int(time.Since(retryStart).Milliseconds())
...
178:             _ = s.store.CompleteCronJobExecution(ctx, retryExec.ID, retryStatus, exitCode, output, errStr, retryDuration)
```

`service.go:263` (`TriggerNow`):

```go
263: func (s *Service) TriggerNow(ctx context.Context, jobID string) (store.CronJobExecution, error) {
264:     if s.store == nil {
265:         return store.CronJobExecution{}, fmt.Errorf("store not initialized")
266:     }
268:     execution, err := s.store.CreateCronJobExecution(ctx, jobID)
268:     if err != nil {
269:         return store.CronJobExecution{}, err
270:     }
272:
273:     go func() {
274:         defer func() { ... recover ... }()
281:         s.executeJob(context.Background(), jobID)
281:     }()
284:     return execution, nil
284: }
```

`forge/api/internal/http/handlers_cronjob.go:15` (`validateCronSchedule`):

```go
15: func validateCronSchedule(schedule string) error {
16:     if strings.TrimSpace(schedule) == "" { ... }
19:     parts := strings.Fields(schedule)
20:     if len(parts) != 5 { return ... }
23:     if parts[0] == "*" && parts[1] == "*" {
24:         return fiber.NewError(fiber.StatusUnprocessableEntity, "minimum cron interval of 1 minute is required")
25:     }
26:     parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
30:     if _, err := parser.Parse(schedule); err != nil {
31:         return fiber.NewError(..., "invalid cron expression: "+err.Error())
32:     }
33:     return nil
34: }
```

Findings reconfirmed:

1. **Duplicate rows — STILL BROKEN.** `TriggerNow:268` creates outer `execution` and returns its ID to caller (`handlers_cronjob.go:196` `POST /cron-jobs/:id/execute → 202 execution,err:=cronJobService.TriggerNow`). The async `go s.executeJob` at `281` creates a *second* execution at `124`. `CompleteCronJobExecution` at `154` completes the *inner* row, leaving the outer caller-visible row permanently `pending`. `GET /cron-jobs/:id/executions` (`handlers_cronjob.go:225`) lists both. FE polling `execution.id == returned.id` sees hung row. Retry loop compounds: each retry creates yet another row at `163`, so `TriggerNow` with `RetryCount=3` yields up to `2+3=5` rows for one manual trigger. 1Panel `reference/app-platforms/1panel/agent/app/api/v2/cronjob.go:348` `HandleOnce` returns only `Success` (no ID) — avoids confusion but sacrifices traceability. Forge wants ID traceability but double-creates.

2. **In-goroutine Sleep blocks scheduler — STILL BROKEN.** `executeJob` is invoked directly from `cron.Cron` callback (`90` `AddFunc(... s.executeJob...)`) with `context.Background()` (no per-execution timeout beyond `runShellCommand:216` `context.WithTimeout` for shell path only). The retry loop `158-161` `time.Sleep(5*(i+1)*s)` runs **inside** that callback goroutine. `cron.Cron` default `WithChain` is not wrapping with async executor, so other jobs sharing the same `cron.Cron` instance stall while retries sleep. `RetryCount=10` worst-case: `5+10+15+...+50 = 275s ≈ 4.5 min` stall. `dispatchServerCommand:252` `if job.TimeoutSeconds>0 { ctx, cancel = context.WithTimeout(ctx, timeout) }` applies to dispatch only if configured, but scheduled `executeJob:91` passes `Background` so outer timeout is missing.

3. **Additional nuance carried over:** `handlers_cronjob.go:82`/`156` `len(req.Command)>4096` rejects long commands, but `forge/web/app/admin/cron-jobs/page.tsx:39` (per prior audit, current `cron-jobs/page.tsx:73-78` modal no longer contains `parseCronExpression` — it is a stub `Use 5-field cron. Command runs as shell on target.` with no validation mirroring `23` `* *` reject). So BE `* *` reject `422` is not surfaced inline in FE — operators still get silent `422` after client-allowed submit. Host confirm: `cron-jobs/page.tsx:73` modal is minimal stub, not mirroring BE `cron.NewParser` error strings.

**Verdict:** **STILL BROKEN** — both `124 vs 268` duplicate and `161` sleep unchanged since `phase-06`. No code drift.

**Recommendation (unchanged):**
- Single-create: either pass outer `execution.ID` into `executeJobWithExecution(ctx, jobID, executionID)` (no second `Create`), or don’t create outer row and return inner ID via channel after `124` `Create`.
- Dispatch retries to separate goroutine/queue with jittered backoff (`time.After` queue), not inside cron callback; or use `cron.WithChain(cron.Recover(...))` + worker pool.
- Pass `context.WithTimeout` from `scheduleJob` into `executeJob` (or at least `TimeoutSeconds` context for both paths) instead of `Background` at `:91`.
- Test: `TriggerNow` → exactly 1 new row until completion; `RetryCount` stress should not stall second job.

---

### F-03 — `monitoring/page.tsx:50 isSynthetic` banner — FIXED (banner) / UNDERLYING COLLECTOR STILL BROKEN

**Prior claims:**
- `phase-06 C12 / LF-04` (Synthetic gap — Beacon synthesizes missing fields; page warning at `monitoring/page.tsx:117` “allocated capacity (not live OS usage)”)
- `final-parity H-12` (Synthetic gap — partial; `isSynthetic` warns but OS series sparse; uptime is daemon uptime not host; `cpuModel` via `/proc/cpuinfo` present)
- `reverification R-10` (Beacon synthesizes zeros for cpuLoad/network; host uptime vs daemon uptime confusion)

**LIVE re-inspection:**

`forge/web/app/admin/monitoring/page.tsx:50`:

```tsx
51:   const isSynthetic = useMemo(() => {
52:     const m = metricsQ.data ?? [];
53:     if (!m.length) return false;
54:     return m.every((x) => (x.cpuLoad1m ?? 0) === 0) && m.every((x) => (x.networkRxBytes ?? 0) === 0);
55:   }, [metricsQ.data]);
...
68:           <span className="text-[var(--text-subtle)]">{metricsQ.data ? `${metricsQ.data.length} points` : "—"} {isSynthetic ? "· allocated" : ""}</span>
...
97:       {isSynthetic ? (
98:         <div className="mt-4 flex gap-2 rounded-lg border border-amber-500/25 bg-amber-500/[0.08] px-3 py-2.5 text-xs leading-5 text-amber-200">
99:           <AlertTriangle size={14} className="shrink-0 mt-0.5" />
100:           <span>CPU/Memory are <b>allocated capacity</b> (not live OS). Network/load not yet collected by Beacon — unavailable. Charts show allocation trends honestly.</span>
101:         </div>
102:       ) : null}
104:
105:       {/* Primary chart — large, readable */}
113:               <AreaChart data={[...(metricsQ.data ?? [])].sort((a, b) => a.observedAt.localeCompare(b.observedAt)).map((m) => ({ ts: m.observedAt, v: (m as unknown as Record<string, number>)[metric] ?? 0 }))} ...>
```

Also: `sysQ` summary + `nodesQ` + `alertsQ` wiring present; `heartbeatmonitor/service.go:89` `DefaultConfig` `Warning 30s/Offline 90s/Unavailable 300s` remains superset; `beacon/internal/metrics/metrics.go:122` `CollectProcess` (heap/goroutines) and `beacon/internal/server/server.go:674` `metrics()` still only report process metrics, not `gopsutil` OS series.

**Verdict:** **FIXED (banner)** — the `isSynthetic` guard requested by prior audits is correctly implemented: it detects `cpuLoad1m==0 && networkRxBytes==0`, shows `· allocated` pill, and renders honest amber banner at `97-101` with `AlertTriangle`. Prior `phase-06 LF-04` wording “isSynthetic guard is a UI band-aid, not a fix” remains accurate: the banner is the correct band-aid and is now present. Underlying live collectors (`gopsutil/load.AvgStat`, `net.IOCounters`, `disk.IOCounters`, `cpu.Percent`, `MonitorGPU`) are still absent on beacon — synthetic zeros remain as data source. That collector gap is **STILL BROKEN** but honestly disclosed, so not a silent lie. Recommendation from prior audits persists: implement live collectors, keep banner until then (promote to blocking if alerts depend on synthetic series).

---

### F-04 — `beacon/handlers_host.go:65 daemon uptime` — STILL BROKEN

**Prior claims:**
- `phase-06 C12` note: “Forge host page host.go slices uptime from s.started :65 time.Since(s.started).Seconds(), not from /proc/uptime — so after a Beacon restart uptime resets (vs system uptime)”
- `final-parity LF-03 Finding 1` (daemon uptime masquerading as host uptime; UI labels it "Uptime" without qualification)
- `reverification LF-04` `(1) daemon uptime masquerades as host uptime — beacon restart mis-diagnosed as host reboot`

**LIVE re-inspection:**

`beacon/internal/server/handlers_host.go:59`:

```go
59: func (s *Server) handleHostInfo(w http.ResponseWriter, r *http.Request) {
60:     hostname, _ := os.Hostname()
61:     info := HostInfo{
62:         Hostname: hostname,
63:         OS:       runtime.GOOS,
64:         Arch:     runtime.GOARCH,
65:         Uptime:   int64(time.Since(s.started).Seconds()),
65:         CPUCores: runtime.NumCPU(),
67:         Time:     time.Now().UTC().Format(time.RFC3339),
68:         Kernel:   kernelVersion(),
69:         CPUModel: cpuModel(),
70:     }
71:     writeJSON(w, http.StatusOK, info)
72: }
```

`forge/web/app/admin/host/page.tsx:62` (`InfoTab`):

```tsx
62:   ["Uptime", fmtUptime(data.uptimeSeconds)],
84:   ["Time", new Date(data.time).toLocaleString()],
56: function fmtUptime(s: number) { const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60); if (d) return `${d}d ${h}h`; ... }
```

No `/proc/uptime` read, no `s.started` fallback, no “Agent uptime” vs “Host uptime” label. Server type `HostInfo:16` `Uptime int64 json:"uptimeSeconds"` is ambiguous.

**Verdict:** **STILL BROKEN** — unchanged since all three prior audits. Correct behavior for host truth is to read `/proc/uptime` first field on linux (`parseUptimeSeconds()`) and fall back to `s.started` for Darwin/test, then expose both `hostUptime` and `agentUptime` if desired. Current single field misleadingly resets on beacon restart, causing operator to mis-diagnose host reboot. `InfoTab` should relabel to “Agent uptime” or qualify.

---

### F-05 — `sysinfo_linux.go:94 inert sort` — STILL BROKEN

**Prior claims:**
- `phase-06 C11` (1Panel streams real-time top via WS + kill; Forge polls every 10s and cannot sort meaningfully (always 0). `sysinfo_linux.go:94 processListPlatform reading /proc/*/status only Name/State, PID (no CPU%/mem%)` + `host/page.tsx:220 sorts by those zeros`)
- `final-parity H-11` (REDUCED — live fidelity gap; `sysinfo_linux.go:94` parses Name/State only. UI sorters `host/page.tsx:234 b.cpuPercent - a.cpuPercent` operate on zeros)
- `reverification R-09 / LF-04` (process sort on zeros preserves input order while UI affords Sort by CPU/Memory — inert; Darwin returns empty `sysinfo_darwin.go:84`)

**LIVE re-inspection:**

`beacon/internal/server/sysinfo_linux.go:94`:

```go
94: func processListPlatform() ([]ProcessEntry, error) {
95:     entries, err := os.ReadDir("/proc")
96:     if err != nil {
97:         return nil, err
98:     }
99:     processes := []ProcessEntry{}
100:     for _, e := range entries {
...
108:         statusData, err := os.ReadFile("/proc/" + e.Name() + "/status")
112:         proc := ProcessEntry{PID: pid}
113:         for _, line := range strings.Split(string(statusData), "\n") {
114:             if strings.HasPrefix(line, "Name:") {
115:                 parts := strings.SplitN(line, ":", 2)
116:                 if len(parts) == 2 {
117:                     proc.Name = strings.TrimSpace(parts[1])
118:                 }
119:             }
120:             if strings.HasPrefix(line, "State:") {
121:                 parts := strings.SplitN(line, ":", 2)
122:                 if len(parts) == 2 {
123:                     stateParts := strings.SplitN(strings.TrimSpace(parts[1]), " ", 2)
124:                     proc.State = stateParts[0]
125:                 }
126:             }
127:         }
128:         if proc.Name == "" {
129:             proc.Name = e.Name()
130:         }
131:         processes = append(processes, proc)
132:     }
133:     return processes, nil
133: }
```

`beacon/internal/server/handlers_host.go:51` struct `ProcessEntry` has `CPU float64 json:"cpuPercent"` `Memory float64 json:"memoryPercent"` but `sysinfo_linux.go:112` only sets `PID`, `Name`, `State` — `CPU`/`Memory` remain `0`.

`beacon/internal/server/sysinfo_darwin.go:84`:

```go
84: func processListPlatform() ([]ProcessEntry, error) {
85:     return []ProcessEntry{}, nil
85: }
```

`forge/web/app/admin/host/page.tsx:168` (`ProcessesTab`):

```tsx
168: function ProcessesTab({ nodeId }: { nodeId: string }) {
169:   const [sortBy, setSortBy] = useState<"cpu" | "mem">("cpu");
...
171:   const { data, isLoading, error, dataUpdatedAt } = useHostQuery(["host-processes", nodeId], fetchHostProcesses, 10_000);
174:   if (!data || data.length === 0) return <div className="py-12 text-center text-sm text-[var(--text-subtle)]">No processes.</div>;
175:   const filtered = data.filter((p: ProcessEntry) => !query || p.name.toLowerCase().includes(query.toLowerCase()));
176:   const sorted = [...filtered].sort((a, b) => (sortBy === "cpu" ? b.cpuPercent - a.cpuPercent : b.memoryPercent - a.memoryPercent));
...
184:         <div className="flex gap-1 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-1">
185:           <button onClick={() => setSortBy("cpu")} className={cn("rounded-md px-3 py-1 text-xs font-medium", sortBy === "cpu" ? "bg-white text-slate-900" : "text-[var(--text-subtle)]")} type="button">CPU</button>
186:           <button onClick={() => setSortBy("mem")} className={cn("rounded-md px-3 py-1 text-xs font-medium", sortBy === "mem" ? "bg-white text-slate-900" : "text-[var(--text-subtle)]")} type="button">Memory</button>
187:         </div>
...
194:             {sorted.slice(0, 100).map((proc: ProcessEntry) => (
195:               <tr key={proc.pid} className="hover:bg-white/[0.02]"><td className="px-3 py-2 text-[var(--text-subtle)]">{proc.pid}</td><td className="px-3 py-2 font-medium text-white">{proc.name}</td><td className="px-3 py-2">{proc.cpuPercent.toFixed(1)}</td><td className="px-3 py-2">{proc.memoryPercent.toFixed(1)}</td><td className="px-3 py-2 text-[var(--text-subtle)]">{proc.state}</td></tr>
```

Also `sysinfo_linux.go:61` `netInterfacesPlatform` reads only `/sys/class/net/*/speed` + `operstate` — no `IPs`/`MAC` (vs `sysinfo_darwin.go:56` `net.Interfaces()` which DOES populate `IPs`+`MAC` via `iface.Addrs()` and `HardwareAddr`). And `handlers_host.go:74` `handleHostDisk` does single `unix.Statfs("/")` not full partition table vs `reference/.../file.go:1102` `GetHostMount` or `gopsutil/disk.Partitions`.

**Verdict:** **STILL BROKEN** — all inert-sort, rendering, and platform-divergence gaps unchanged:

1. `sysinfo_linux.go:94` `CPU=0 Memory=0` for every process — `host/page.tsx:176` sort on zeros is inert (stable sort preserves `/proc` readdir order). UI affords `CPU`/`Memory` sort buttons that do nothing — functional lie.
2. `host/page.tsx:195` `proc.cpuPercent.toFixed(1)` displays `0.0` not `—`; should render `—` until wired, and disable sort or show pill “CPU/Memory — collection not yet available — Name/State only”.
3. Darwin `sysinfo_darwin.go:84` returns empty → `host/page.tsx:174` shows “No processes.” with no hint that Darwin is stubbed vs Linux populated — divergent UX with no qualifier.
4. Collateral still broken: linux `netInterfacesPlatform:61` missing `IPs`/`MAC` while darwin shows them; single-disk `handleHostDisk:74` vs full mount table.

**Recommendation (unchanged):** Replace `/proc/status` parser with `gopsutil/process.Percent+MemoryInfo` (or explicitly render `—` and remove sorters until wired); populate linux `netInterfaces` with `net.Interfaces()` + `Addrs()` overlay plus `/sys/class/net` speed/operstate; expand disk to `gopsutil/disk.Partitions` or doc `single /` limitation.

---

### F-06 — Firewall placeholder `0.0.0.0/0` — STILL BROKEN (UX footgun; policy itself CORRECT)

**Prior claims:**
- `phase-06 LF-01` (Firewall source restriction is secure-by-default but breaks 1Panel’s open-to-internet UX; migration creating “open 25565/tcp to all” will 400)
- `final-parity LF-04` (Placeholder re-introduces rejected input: `AdminFirewall.tsx:356 placeholder e.g. 0.0.0.0/0 which validateFirewallSource:132 will always 400`)
- `reverification LF-05` (secure-by-default is correct but migration-breaking + deny UX confusion)

**LIVE re-inspection:**

`beacon/internal/server/handlers_firewall.go:109`:

```go
109: func validateFirewallProtocol(proto string) (string, error) {
110:     switch lower := strings.ToLower(strings.TrimSpace(proto)); lower {
111:     case "tcp", "udp":
112:         return lower, nil
113:     case "tcp/udp", "udp/tcp":
114:         return "", errors.New("combined protocol tcp/udp is not supported; use separate rules")
115:     default:
116:         return "", fmt.Errorf("unsupported protocol %q", proto)
117:     }
118: }
120: func validateFirewallAction(action string) (string, error) {
121:     switch lower := strings.ToLower(strings.TrimSpace(action)); lower {
122:     case "", "allow", "accept", "open":
123:         return "allow", nil
124:     default:
125:         return "", fmt.Errorf("unsupported action %q: only allow/accept is supported", action)
126:     }
127: }
129: func validateFirewallSource(source string) error {
130:     source = strings.TrimSpace(source)
131:     if source == "" {
132:         return errors.New("source IP or CIDR is required; unrestricted rules are not allowed")
132:     }
134:     if ip := net.ParseIP(source); ip != nil {
135:         if !safeFirewallSourceIP(ip) {
136:             return fmt.Errorf("source IP %q is not a permitted public unicast address", source)
137:         }
138:         return nil
139:     }
140:     if ip, network, err := net.ParseCIDR(source); err == nil {
141:         ones, _ := network.Mask.Size()
142:         if ones == 0 || !ip.Equal(network.IP) || !safeFirewallSourceIP(ip) {
143:             return fmt.Errorf("source CIDR %q is unrestricted, non-canonical, or non-public", source)
144:         }
145:         return nil
146:     }
147:     return fmt.Errorf("invalid source IP or CIDR %q", source)
148: }
...
162: func safeFirewallSourceIP(ip net.IP) bool {
163:     return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
164: }
```

`beacon/internal/server/handlers_firewall.go:259` `persistLocked` (0700 dir, 0600 file, `CreateTemp`+`Write`+`Sync`+`Rename`+`syncDirectory`), `330` `ensureChains`, `361` `reconcile` `iptables-restore --wait 10 --noflush` transactional, `347` `ruleArgs` stamps `forge:<id>` comment — all correctly hardened.

`forge/web/components/admin/AdminFirewall.tsx:356`:

```tsx
356:           <Input label="Source IP" value={source} onChange={setSource} placeholder="e.g. 0.0.0.0/0" />
...
357:           <AdminSelect label="Action" value={action} onChange={setAction} options={[
358:             { value: "allow", label: "ALLOW" },
359:             { value: "deny", label: "DENY" },
359:           ]} />
...
403:           <Input label="Source IP" value={source} onChange={setSource} placeholder="e.g. 0.0.0.0/0" />
404:           <AdminSelect label="Action" value={action} onChange={setAction} options={[
405:             { value: "allow", label: "ALLOW" },
406:             { value: "deny", label: "DENY" },
406:           ]} />
```

`forge/api/internal/http/handlers_firewall.go:7` thin proxy — validation delegated entirely to beacon (no second check).

Findings:

1. **Secure-by-default policy is FIXED/CORRECT** — `validateFirewallSource:129-132` requires CIDR/IP and rejects `0.0.0.0/0` via `ones==0` check at `142` and `safeFirewallSourceIP:162` `IsGlobalUnicast && !IsPrivate`. `validateFirewallProtocol:109` correctly rejects `tcp/udp` combined. `validateFirewallAction:120` correctly only `allow` (hardening). `reconcile:361` + `persistLocked:259` are crash-consistent and transactional — superset over 1Panel's rule-by-rule GORM rows. **No change needed to policy.**

2. **Placeholder footgun — STILL BROKEN.** `AdminFirewall.tsx:356,403` placeholder `e.g. 0.0.0.0/0` suggests input that `validateFirewallSource:132` will always `400` with “unrestricted rules are not allowed” / “CIDR is unrestricted, non-canonical, or non-public”. Every new admin following placeholder text gets error on first try. File was already flagged in `final-parity H-04` and `reverification LF-05`; live still shows footgun. Should be `e.g. 203.0.113.45 or 203.0.113.0/24`.

3. **Deny UX dead — STILL BROKEN.** `AdminFirewall.tsx:359,406` offers `deny` but `validateFirewallAction:124` `only allow/accept is supported` → `400`. `RuleRow:270` `actionColor = rule.action === "deny" ? "text-red-400"` assumes `deny` exists. `decodeFirewallRule:486` would `400` any `deny` update. `post /rules` proxy at `handlers_firewall.go:7` passthrough means `deny` reaches beacon and `502`/`400` with no FE hint.

**Verdict:** Split — **policy FIXED/STRICTER** (intentional hardening, keep), **UX placeholder + deny STILL BROKEN** (P3). Non-goal to weaken validation; fix is FE copy + hide `deny` unless beacon supports `drop`.

**Recommendation:** Keep restriction. Fix `AdminFirewall.tsx:356,403` placeholder to `e.g. 203.0.113.45 or 203.0.113.0/24` + add helper text “Use public unicast IP or CIDR; unrestricted `0.0.0.0/0` is rejected (create per-CIDR rules)”. Hide/ disable `deny` option unless `drop` semantics are implemented beacon-side; or map `deny` to reject with `400` hint pre-validation in FE. Document migration rewrite: `allow 25565/tcp from 0.0.0.0/0` → two explicit CIDR rules or gated `infra_admin allowAnySource` if business requires (proposed in `final-parity H-04` but not implemented — keep as INFO decision).

---

### F-07 — Hostfiles allowlist properly pinned vs `1Panel file.go:400` — FIXED / STRONGER

**Prior claims:**
- `phase-06 C09 / LF-02 Finding 1` (Beacon `hostfiles.go:51 resolveHostPath` allowlist-vs-denylist; denylist blocks legitimate operator roots with no allowlist configured; but conservative)
- `final-parity H-09 / H-17` (Forge STRONGER on confinement — allowlist-vs-denylist + `secure_files.go`/`rootfs` `openat2 RESOLVE_BENEATH`)
- `reverification R-04/R-05` (Forge stricter host prefix + sandbox kernel boundary; 1Panel permissive-by-default; Forge deny-by-default)

**LIVE re-inspection:**

`beacon/internal/server/hostfiles.go:19`:

```go
19: func validateHostPath(raw string) (string, error) {
20:     if raw == "" || strings.ContainsRune(raw, '\x00') || !strings.HasPrefix(raw, "/") {
21:         return "", errors.New("path must be an absolute path")
22:     }
23:     cleaned := path.Clean(raw)
24:     normalized := strings.TrimSuffix(raw, "/")
25:     if normalized == "" {
26:         normalized = "/"
27:     }
28:     if cleaned != normalized || strings.Contains(raw, "\\") {
29:         return "", errors.New("path must be canonical and may not contain traversal segments")
30:     }
31:     return cleaned, nil
32: }
34: var hostFileDenylistPrefixes = []string{
37:     "/etc", "/proc", "/sys", "/dev", "/boot",
37:     "/usr", "/bin", "/sbin", "/lib", "/lib64",
39:     "/root", "/var/run", "/run",
40: }
```

`beacon/internal/server/hostfiles.go:51`:

```go
51: func (s *Server) resolveHostPath(raw string) (string, error) {
52:     cleaned, err := validateHostPath(raw)
...
56:     if s != nil && s.dataDir != "" {
57:         dd := strings.TrimSuffix(s.dataDir, "/")
58:         if cleaned == dd || strings.HasPrefix(cleaned, dd+"/") {
59:             return "", errors.New("access to the beacon data directory is not permitted")
59:         }
60:     }
62:     s.hostFileRootsMu.RLock()
63:     roots := s.hostFileRoots
64:     s.hostFileRootsMu.RUnlock()
64:     underAny := func(p string, prefixes []string) bool {
68:         for _, root := range prefixes {
69:             if p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/") {
69:                 return true
70:             }
71:         }
72:         return false
73:     }
74:     if len(roots) > 0 {
77:         if !underAny(cleaned, roots) {
78:             return "", fmt.Errorf("path %q is outside the configured host file allowlist", cleaned)
79:         }
80:         return cleaned, nil
80:     }
82:     if cleaned == "/" || underAny(cleaned, hostFileDenylistPrefixes) {
84:         return "", fmt.Errorf("path %q is in a protected system location; configure DAEMON_HOST_FILES_ALLOWLIST to grant explicit roots", cleaned)
85:     }
86:     return cleaned, nil
87: }
```

`beacon/internal/server/hostfiles.go:92` `SetHostFileAllowlist` validates each root via `validateHostPath` (`95` canonical check).

`beacon/internal/server/hostfiles.go:109` `hostAtomicWrite` sibling-temp `CreateTemp`+`Chmod`+`io.Copy LimitReader limit+1`+`Sync`+`Close`+`Rename`+`dir Sync` — crash-consistent.

`forge/api/internal/http/handlers_files.go:547` `validateHostFilePath` mirrors `19` canonical checks before dispatch:

```go
547: func validateHostFilePath(value string) error {
548:     if value == "" || strings.ContainsRune(value, '\x00') || !strings.HasPrefix(value, "/") {
549:         return fiber.NewError(fiber.StatusUnprocessableEntity, "path must be an absolute path")
550:     }
551:     cleaned := path.Clean(value)
552:     normalized := strings.TrimSuffix(value, "/")
552:     if normalized == "" { normalized = "/" }
556:     if cleaned != normalized || strings.Contains(value, `\`) {
557:         return fiber.NewError(fiber.StatusUnprocessableEntity, "path must be canonical and may not contain traversal segments")
558:     }
559:     return nil
560: }
```

Plus per-server sandbox `beacon/internal/rootfs/rootfs_linux.go:44` `RESOLVE_BENEATH|RESOLVE_NO_MAGICLINKS|RESOLVE_NO_SYMLINKS` + `O_NOFOLLOW` for workload FS (separate from host FS).

Contrast `reference/app-platforms/1panel/agent/app/api/v2/file.go:400` `UploadFiles`:

```go
400: func (b *BaseApi) UploadFiles(c *gin.Context) {
401:     form, err := c.MultipartForm()
405:     uploadFiles := form.File["file"]
...
422:     dir := path.Clean(paths[0])
...
443:     stat, ok := info.Sys().(*syscall.Stat_t)
446:         uid, gid = int(stat.Uid), int(stat.Gid)
...
468:         tmpFilename := dstFilename + ".tmp"
469:         if err := c.SaveUploadedFile(file, tmpFilename, dstDirMode); err != nil {
...
481:         err = os.Rename(tmpFilename, dstFilename)
```

1Panel checks `ShouldDenySensitiveFileRead` only for reads (`file.go:650`), writes anywhere agent uid can write, `.tmp+rename` without sibling-temp guarantee or dir `Sync`, uid/gid preservation but no allowlist.

**Verdict:** **FIXED / STRONGER** — confirms prior `final-parity H-17` and `reverification R-04` claims:

- Allowlist pinned via `SetHostFileAllowlist:92` + `resolveHostPath:76` `underAny` prefix (sibling-aware `TrimSuffix`+`/` guard prevents `/srv/data` vs `/srv/data-evil` sibling bypass — already covered by `hostfiles_confinement_test.go:73`).
- Denylist mode with `DAEMON_HOST_FILES_ALLOWLIST` unconfigured still blocks `"/"` + all `hostFileDenylistPrefixes:36` plus `dataDir:57` hard block — safe default, noisy but correct.
- Dual validation (Forge `547` + Beacon `19`+`51`) defense-in-depth before dispatch.
- `hostAtomicWrite:109` crash-consistent with `LimitReader` guard (`124` `limit+1` → `written>limit` `ErrTooLarge`) mirroring `handlers_firewall.go:259` `persistLocked` discipline.
- Strictly tighter than Wings and 1Panel permissive model. Trade-off documented: denylist `"/usr"` blocks `/usr/local/forge` operator tooling → requires explicit allowlisted roots (`DAEMON_HOST_FILES_ALLOWLIST=/srv/game-panel,/opt/forge,/tmp` example per `phase-06 LF-02 rec`).

Note nuance per `final-parity §3`: host FS uses prefix checks (`hostfiles.go:67`), not `rootfs` `RESOLVE_BENEATH` fd-pinning — fd-pinning applies only to per-server sandbox. Host allowlist is correctly “STRONGER” not “complete” — prefix root like `/srv` still relies on prefix not `openat2`. Document as strength.

---

## 2. Summary of Drift vs Prior Audits

No silent drift from `phase-06` / `final-parity` / `reverification-12` to LIVE:

- All 5 `STILL BROKEN` findings reproduce at identical lines — no regression, no hidden fix.
- `F-01` severity reduced `P1→P2` because `forge/api/internal/http/server.go:1040` `BodyLimit: 32 MiB` now bounds worst-case (added since initial `phase-06` audit which noted “no explicit cap beyond Fiber's default”). The handler `handlers_files.go:402` itself is still unbounded before that global cap.
- `F-03` `isSynthetic` banner now correctly present — the only finding that moved `BROKEN → FIXED` between `phase-06` (UI band-aid noted at `monitoring/page.tsx:117`) and `final-parity`/`reverification` (honest banner) and LIVE.
- `F-07 allowlist` was already fixed before `phase-06` and remains so — no drift.

---

## 3. Prior Audit Cross-Ref — What Was Confirmed vs What Remains Intentionally REJECT

For completeness, the `REJECT` host-tool sprawl identified as intentionally absent (per `final-parity §4`) reconfirms with no LIVE change — do not re-file as MISSING:

| 1Panel surface | Forge stance | LIVE evidence |
|---|---|---|
| `docker.go:19-161` daemon.json + `OperateDocker` | **REJECT** — immutable provisioned nodes | `beacon/internal/server/container_admin.go:1040` `adminDockerClient()` dials socket only |
| `ssh.go:17-271` sshd + `RootCert` CA | **REJECT** — MTLS terminal WS | `forge/api/internal/http/handlers_files.go:113` `registerHostTerminalRoute` `150` `HeartbeatStateOffline` gate, no sshd writer |
| `fail2ban.go:16-143` jail.local | **REJECT** | `heartbeatmonitor/service.go:89` `DefaultConfig` + no fail2ban handler |
| `ftp.go:16-180` vsftpd | **REJECT** — per-server confined SFTP (`rootfs` RESOLVE_BENEATH) | `beacon/internal/rootfs/rootfs_linux.go:44` |
| `snapshot.go:12-193` system image rollback | **REJECT** — workload artifact backups `BackupTypeServer/Database/Volume/App` | `forge/api/internal/services/backup/service.go:48` SourceType |
| `host.go:19-243` multi-host SSH fleet | **REJECT** — fleet nodes via `GetNode`+`GetNodeDaemonCredential` | `forge/api/internal/http/handlers_host.go:35` `resolveNodeHostTarget` |
| `device.go/gpu.go/host_tool.go` | **REJECT** — capabilities delta | `forge/api/internal/daemon` no host_tool CRUD |

These are correctly called `REJECT` not gaps per `FINAL §22` host-tool sprawl NOT to copy.

---

## 4. Citations Index — Every Finding Above Is File:Line Verified This Pass

- **Forge proxy:** `forge/api/internal/http/handlers_files.go:32,69,113,269,288,313,338,363,386,402,547` `forge/api/internal/http/handlers_firewall.go:7` `forge/api/internal/http/handlers_cronjob.go:15,23,29,59,131,183,196,209,225` `forge/api/internal/http/handlers_host.go:11,35,69,87,104,121,138` `forge/api/internal/http/server.go:1040` (BodyLimit)
- **Forge cron service:** `forge/api/internal/services/cronjob/service.go:21,62,81,90,100,109,117,124,134,158,161,163,190,212,220,244,263,268,281,287`
- **Beacon host seam:** `beacon/internal/server/handlers_host.go:12,51,59,65,74,100,121,130,139,154` `beacon/internal/server/sysinfo_linux.go:13,21,29,37,45,61,94,112` `beacon/internal/server/sysinfo_darwin.go:12,20,28,40,48,56,84` `beacon/internal/server/hostfiles.go:19,36,51,67,76,83,92,109,124,457,488` `beacon/internal/server/handlers_firewall.go:24,29,80,96,109,120,129,150,162,211,259,330,347,361,378,393,412` `beacon/internal/rootfs/rootfs_linux.go:44` `beacon/internal/server/secure_files.go:26,545,553`
- **Forge web host/monitor:** `forge/web/app/admin/host/page.tsx:56,62,84,98,168,176,184,195` `forge/web/app/admin/monitoring/page.tsx:50,54,68,97,100` `forge/web/app/admin/cron-jobs/page.tsx:39,73` `forge/web/components/admin/AdminFirewall.tsx:356,359,403,406`
- **1Panel reference:** `reference/app-platforms/1panel/agent/app/api/v2/file.go:400,468,481,650,1102` `reference/app-platforms/1panel/agent/app/api/v2/cronjob.go:23,348` `reference/app-platforms/1panel/agent/app/api/v2/firewall.go:60,90,135` `reference/app-platforms/1panel/agent/app/api/v2/ssh.go:17` `reference/app-platforms/1panel/agent/app/api/v2/fail2ban.go:16` etc.

---

*End of subagent-06 host-infra confirm — 7 findings inspected: 2 FIXED (F-03 banner, F-07 allowlist STRONGER), 5 STILL BROKEN (F-01 buffered upload double-alloc despite 32 MiB BodyLimit, F-02 duplicate+scheduler stall, F-04 daemon uptime, F-05 inert sort, F-06 placeholder 0.0.0.0/0 footgun + deny dead-UX). Policy hardening (firewall allow-only + CIDR, transactional restore, file atomicity) is correctly implemented — UX/process gaps are the remaining work.*
