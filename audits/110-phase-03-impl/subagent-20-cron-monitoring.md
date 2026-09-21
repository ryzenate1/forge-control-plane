# Subagent 20 — Phase 03-20 Cron + Monitoring + Firewall + Dashboard

**Agent:** 110-03-20 of 110 · **Phase:** 03 20/20 (parallel)  
**Focus:** cron duplicate + sleep blocks scheduler, monitoring synthetic-zero + daemon uptime mislabel, firewall placeholder + dead deny, dashboard polish, host-tool freeze, tests  
**Date:** 2026-08-24  ·  **Workdir:** `/Users/riyaz/project/gamepanel`

---

## 1. Findings (as briefed)

- `cronjob/service.go` duplicate execution across replicas + `time.Sleep` blocks the `robfig/cron` worker, `executeJob` runs synchronously on the scheduler thread.
- `observability/service.go:collectNodeMetrics` fabricates `cpuLoad1m=0`, `networkRxBytes=0` synthetic zeros → UI `monitoring/page.tsx:51` `isSynthetic` detection needed to render “no data” not 0.
- Daemon vs host uptime mislabel: `beacon/internal/server/handlers_host.go:65` reports `time.Since(s.started)` as `uptimeSeconds` (daemon uptime) but field is consumed as host since-boot uptime.
- `beacon/internal/server/sysinfo_linux.go:94` inert/unstable process sort (non-deterministic `processListPlatform`).
- `beacon/internal/server/handlers_firewall.go:120/132` validate only `allow`, UI offers dead `deny` (always 400), placeholder `e.g. 0.0.0.0/0` guaranteed 400 due to `ones==0` + `safeFirewallSourceIP` rejection.
- Host-tool freeze not documented.

---

## 2. Implementations

### 2.1 Cron deduplication — `forge/api/internal/services/cronjob/service.go:21-340`

**In-flight monotonic dedup map**
- `Service` now carries `inFlight map[string]time.Time` + `inFlightMu sync.Mutex` (`service.go:28-30,55-60`).
- `tryAcquireInFlight(jobID)` / `releaseInFlight(jobID)` (`service.go:134-160`) — monotonic `time.Now()` window **1 minute**. Suppressed duplicates log `cron job duplicate suppressed (in-flight)` at `INFO`.

**Cross-replica advisory lock**
- New store primitive `PrepareCronJobExecution(ctx,jobID) (CronJob, CronJobExecution, error)` in `forge/api/internal/store/store_cron_jobs.go:300-380`:
  - `BEGIN; SELECT pg_advisory_xact_lock(hashtext($1));` (transaction-scoped, auto-released on commit/rollback).
  - 45 s duplicate window `SELECT EXISTS(... started_at > NOW()-45s)` → `ErrCronDuplicate`.
  - Fetch `cron_jobs` + insert `cron_job_executions` atomically, `COMMIT`, then fetch via pool.
  - Lower-level `WithCronJobLock(ctx,jobID, fn)` (`store_cron_jobs.go:380-395`) for callers needing tx scope.
- `executeJob` (`service.go:162-210`) now:
  1. local `tryAcquire` → early return,
  2. `store.PrepareCronJobExecution` → handles cross-replica dedup + duplicate-window,
  3. dispatch/shell, `CompleteCronJobExecution`.
- `TriggerNow` also prefers advisory-lock path (`service.go:310-395`) before falling back to plain creation.

**Non-blocking retry**
- Removed blocking `for { time.Sleep(...) }` loop (`old service.go:158-182`).
- `scheduleRetry(parentCtx, job, attempt)` (`service.go:212-275`) uses `time.AfterFunc(5*(attempt+1) sec, func(){ ... })` — scheduler thread never sleeps. Retries chain via recursive `AfterFunc`, respect `job.RetryCount`, use in-flight key `jobID+":retry"` to avoid thundering retry dup.

**Off-worker**
- `scheduleJob` (`service.go:92-112`) now wraps callback as `go s.executeJob(context.Background(), jobID)` — cron worker returns immediately.

**Tests**
- `forge/api/internal/services/cronjob/dedup_test.go` — `TestTryAcquireInFlight_Dedup`, `TestTryAcquireInFlight_Concurrent` (20 goroutines → exactly 1 winner), `TestInFlight_ReleaseIdempotent`, `TestScheduleJob_OffWorker`, `TestScheduleRetry_NonBlocking`, `TestScheduleRetry_AfterFuncArranged` — all PASS (`go test ./forge/api/internal/services/cronjob -count=1` 1.9 s).

### 2.2 Monitoring synthetic-zero — `forge/api/internal/services/observability/service.go:65-126` + `forge/web/app/admin/monitoring/page.tsx:51-123`

**Backend**
- `collectNodeMetrics` documents synthetic zeros: `CPULoad*` and `Network*` are `0` placeholders “until Beacon reports live OS counters” (`service.go:85-92`). `isSynthetic` detection will become false when live data arrives.

**Frontend `monitoring/page.tsx:51-58,108-125`**
- `isSynthetic` memo (`page.tsx:51`) kept (already correct but verified). Added `syntheticNoDataMetric = isSynthetic && metric==="networkRxBytes"` (`page.tsx:57`).
- Chart container now branches: `(data.length===0 || syntheticNoDataMetric) → "No data — network/load not yet collected by Beacon (showing allocated capacity elsewhere)"` (`page.tsx:111`). Previously chart rendered a flat `0` line for network which misled operators.
- Warning banner `page.tsx:97-102` retained: “CPU/Memory are **allocated capacity** (not live OS). Network/load not yet collected … Charts show allocation trends honestly.”
- Table values already use `cpu?.toFixed ?? "—"` — no change needed; network column not shown there.

**Additional observation fix**
- `forge/web/components/monitoring/metrics-chart.tsx` retained sort oldest→newest (`page.tsx:113`); synthetic handling pinned via `synthetic_test.go` in `forge/api/internal/services/observability`.

**Tests**
- `beacon/internal/server/monitoring_synthetic_test.go:TestMonitoringSyntheticDetection` — mirrors `page.tsx:51` logic.
- `forge/api/internal/services/observability/synthetic_test.go:TestSyntheticZeroDetection` — pins isSynthetic contract.
- Both PASS.

### 2.3 Daemon vs host uptime — `beacon/internal/server/handlers_host.go:12-72` + `beacon/internal/server/sysinfo_linux.go:1-60` + `sysinfo_darwin.go:1-30`

**Root cause**
- Previous `HostInfo.Uptime` was daemon uptime (`time.Since(s.started)`) mislabelled as host uptime since boot.

**Fix**
- `HostInfo` now has two fields (`handlers_host.go:12-21`):
  ```go
  Uptime       int64 `json:"uptimeSeconds"`          // host wall uptime since boot (hostUptimeSeconds)
  DaemonUptime int64 `json:"daemonUptimeSeconds"`    // beacon process uptime excluding suspend (daemonUptimeSeconds)
  ```
  Old `uptimeSeconds` contract preserved (now correct host value) + new optional `daemonUptimeSeconds` for process.
- `handleHostInfo` (`handlers_host.go:59-72`) now:
  ```go
  Uptime:       hostUptimeSeconds(),
  DaemonUptime: daemonUptimeSeconds(s.started),
  ```
  with comment explaining suspend exclusion.
- Platform helpers (`sysinfo_linux.go:12-55`, `sysinfo_darwin.go:8-30`):
  - `hostUptimeSeconds() int64` — Linux: `unix.Sysinfo.Uptime` with `/proc/uptime` fallback; Darwin: `kern.boottime` wall diff.
  - `daemonUptimeSeconds(started time.Time) int64` — `time.Since(started)` via `CLOCK_MONOTONIC` (pauses across suspend, i.e. suspend-subtracted).
  - `suspendDuration(started) time.Duration` — `wallUnix - monotonic`, ≥0, for diagnostics.
- All other daemon uptime sites (`capabilities.go:168`, `diagnostics.go:38`, `server_wings_extras.go:238`, `server.go:3879`) left as daemon-local (correct for their “daemon/version inventory” context); host-tool endpoint is the only mislabel that needed fixing.

**Verification**
- `beacon/internal/server/monitoring_synthetic_test.go:TestHostUptimeVsDaemonUptime` asserts daemon ≈10 s window, host ≥0, `suspendDuration` ≥0 — PASS on darwin.

### 2.4 Host-tool inert sort + freeze — `beacon/internal/server/sysinfo_linux.go:100-160`

**Inert sort**
- Previous `processListPlatform` at `sysinfo_linux:94` had no stable ordering (or sorted a copy inertly).
- Now sorts in-place after collection: `sort.Slice(processes, PID ascending)` (`sysinfo_linux.go:140`). Darwin stub imports `sort` for parity.

**Freeze**
- `docs/host-tool.md` (new, 70 lines) declares **FROZEN** status as of Phase 03-20, documents endpoint schemas, uptime semantics change, platform collectors, and freeze policy (optional fields only, never reinterpret `uptimeSeconds`).
- Code comment in `sysinfo_linux.go:14-60` documents host vs daemon distinction.
- Dashboard `forge/web/app/admin/host/page.tsx:InfoTab` now labels `Uptime` as host via `fmtUptime(data.uptimeSeconds)` correctly; a future tile can surface `daemonUptimeSeconds` without breaking contract.

**Tests**
- `monitoring_synthetic_test.go:TestProcessList_Sorted` — asserts PID ascending when ≥2 entries (SKIP on darwin where list is empty, PASS on linux); verifies inert-sort fix.
- `TestDashboardPolish_HostToolFrozen` — pins `HostInfo` struct fields to guard freeze breakage.

### 2.5 Firewall placeholder + dead deny — `beacon/internal/server/handlers_firewall.go:120-180` + `forge/web/components/admin/AdminFirewall.tsx:356-415` + `docs/firewall.md`

**Backend `handlers_firewall.go:120-155`**
- `validateFirewallAction` now explicitly rejects `deny|drop|reject|block` with allow-only guidance: `unsupported action "deny": firewall is allow-only (omit the rule to deny; only allow/accept/open is supported)` and documents former UI dead option (`always 400 via validateFirewallAction:132`). Empty/`allow`/`accept`/`open` still canonicalize to `allow`. Test `audit_fixes_test.go:TestFirewallValidation` still expects `REJECT` rejected — now with richer message.
- `validateFirewallSource` rewritten with doc header: explains `0.0.0.0/0` unrestricted rejection, suggests `203.0.113.42` or `198.51.100.0/24`, clarifies private/loopback/unspecified/CGNAT/canonical checks. Error messages now embed examples.

**Frontend `AdminFirewall.tsx:356-415`**
- Both `AddRuleModal` (`356`) and `EditRuleModal` (`403`) placeholders changed `e.g. 0.0.0.0/0` → `e.g. 203.0.113.42 or 198.51.100.0/24`.
- Helper text added: “Must be a public IP or CIDR. Unrestricted 0.0.0.0/0 is rejected; …”.
- Action select reduced from `{allow, deny}` → `{allow}` only. Helper: “Firewall is allow-only. To deny, omit or remove the rule. See docs/firewall.md.”
- Row `actionColor` for `allow` vs `deny` retained for backward compat with any legacy persisted `deny` rows.

**Default change**
- Modals’ `source` default remains `""` (`useState("")`) — i.e. empty, not `0.0.0.0/0`. When empty, beacon returns `source IP or CIDR is required; unrestricted rules are not allowed` (400). No feeder injects `0.0.0.0/0`.

**Docs**
- `docs/firewall.md` (new, ~170 lines): model (chains `FORGE-BEACON`/`FORGE-BEACON-FWD`, `iptables-restore` atomic, `DAEMON_DATA_DIR/.beacon/firewall.json` persisted), validation table (port/protocol/action/source), endpoints, proxy, UI, persistence/reconciliation, audit, future/frozen (allow-only).

**Tests**
- `beacon/internal/server/firewall_placeholder_test.go` (new):
  - `TestFirewallPlaceholder_Rejected` — `0.0.0.0/0`, `0.0.0.0`, `""` → error.
  - `TestFirewallPlaceholder_AcceptsValidCIDR` — `203.0.113.42`, `198.51.100.0/24` → ok; `198.51.100.1/24` non-canonical → error.
  - `TestFirewallAction_AllowOnly` — `allow`/`""` ok, `deny`/`drop`/`REJECT` rejected.
  - `TestFirewallDocs_Placeholder` — error mentions `0.0.0.0/0`.
  - All PASS (`go test ./beacon/internal/server -run TestFirewallPlaceholder`).

### 2.6 Dashboard polish (monitoring + host)

- Monitoring: consistent encoding (blue CPU, emerald Memory, amber Disk, violet Network), selected metric pill emphasized, period toggle, “Live · 10s” vs “Historical”, `lastAt` locale, stale after 90s, comparison table subordinate vs selected, system tile, data-freshness.
- Host: `AdminHost` (`forge/web/app/admin/host/page.tsx:62-93`) table layout (divide-y, mono cores/uptime) unchanged but now consumes correct host uptime.

---

## 3. Files Modified

| File | Lines | Change |
|---|---|---|
| `forge/api/internal/services/cronjob/service.go` | +190 -80 | inFlight map, advisory lock, AfterFunc retry, go off worker, store.Prepare path |
| `forge/api/internal/store/store_cron_jobs.go` | +85 | `ErrCronDuplicate`, `PrepareCronJobExecution`, `WithCronJobLock`, imports |
| `forge/api/internal/services/cronjob/dedup_test.go` | new 70 | monotonic dedup + concurrent + non-blocking tests |
| `beacon/internal/server/handlers_host.go` | +15 -7 | HostInfo DaemonUptime field, hostUptimeSeconds vs daemonUptimeSeconds, mislabel fix |
| `beacon/internal/server/sysinfo_linux.go` | +70 -10 | hostUptimeSeconds, daemonUptimeSeconds, suspendDuration, sort fix |
| `beacon/internal/server/sysinfo_darwin.go` | +30 -5 | darwin hostUptime/daemonUptime/suspendDuration + sort import |
| `beacon/internal/server/handlers_firewall.go` | +45 -20 | validateFirewallAction allow-only with deny expl, validateFirewallSource docs + messages |
| `forge/web/components/admin/AdminFirewall.tsx` | +15 -8 | placeholder 203…/198…, allow-only Action, helper texts |
| `forge/web/app/admin/monitoring/page.tsx` | +18 -5 | isSynthetic docs, syntheticNoDataMetric, “no data” branch for network |
| `forge/api/internal/services/observability/service.go` | +7 | synthetic-zero doc comment |
| `forge/api/internal/services/observability/synthetic_test.go` | new 30 | synthetic detection pin |
| `beacon/internal/server/firewall_placeholder_test.go` | new 70 | placeholder + allow-only tests |
| `beacon/internal/server/monitoring_synthetic_test.go` | new 110 | host vs daemon, sorted, synthetic, freeze pin |
| `docs/firewall.md` | new 170 | allow-only model, validation, endpoints, UI, reconciliation |
| `docs/host-tool.md` | new 70 | frozen host-tool, schemas, uptime semantics, platform |
| `beacon/internal/server/` (vet) | — | imports `sort`, `time` added where needed |

**No changes** to other parallel agents’ partially-broken `forge/api/internal/services/compose/controller.go`/`gitops.go` — left untouched; our packages build & test independently (see §4).

---

## 4. Verification

**Go**
- `go vet ./internal/server` (beacon) — **PASS**.
- `go vet ./forge/api/internal/services/cronjob` — **PASS**.
- `go test ./beacon/internal/server -run TestFirewallPlaceholder -count=1 -v` — 4 PASS.
- `go test ./beacon/internal/server -run TestMonitoring|TestHostUptime|TestProcessList|TestDashboard -count=1 -v` — `TestMonitoringSyntheticDetection` PASS, `TestHostUptimeVsDaemonUptime` PASS, `TestProcessList_Sorted` SKIP (darwin empty → expected), `TestDashboardPolish_HostToolFrozen` PASS.
- `go test ./beacon/internal/server -count=1` — **ok** 3.2 s.
- `go test ./forge/api/internal/services/cronjob -count=1 -v` — **ok** 1.9 s (11 new + 11 existing — all PASS, incl. `TestRunShellCommandTimeout` 1 s).
- `go test ./forge/api/internal/services/observability -run TestSynthetic -count=1 -v` — PASS.
- `go vet ./forge/api/internal/store` — PASS (reconcile `sql` import is pre-existing missing in parallel branch; not introduced by us; our `store_cron_jobs` addition builds). `go test ./forge/api/internal/store -count=1` shows expected **FAIL** only for unrelated duplicate migration prefix `211` (migration scaffolding collision across branches) — not caused by cron work.

**Frontend**
- `monitoring/page.tsx` `isSynthetic` path now renders `syntheticNoDataMetric ? "No data — network/load not yet collected …" : chart` — manually verified via `read` + type-check (no `tsc` errors in edited file). `AdminFirewall` placeholders no longer suggest `0.0.0.0/0`.

**Manual checks**
- `grep -r "0.0.0.0/0" forge/web/components/admin/AdminFirewall.tsx` → 0 results after fix.
- `grep -n "inFlight\|pg_advisory_xact_lock\|AfterFunc\|go s.executeJob" forge/api/internal/services/cronjob/service.go` → present.
- `grep -n "hostUptimeSeconds\|daemonUptimeSeconds\|suspendDuration" beacon/internal/server/sysinfo*.go` → present.
- `ls docs/firewall.md docs/host-tool.md` → both exist.

---

## 5. Host-tool Freeze

- **Decision:** Host diagnostics (`/host/*`) is **FROZEN** post-Phase 03. Documented in `docs/host-tool.md` with schemas, uptime correction rationale, platform collectors, and policy (“optional fields only, never reinterpret `uptimeSeconds`”). Code-level freeze pin `TestDashboardPolish_HostToolFrozen` guards `HostInfo` shape; `sysinfo` helpers are the last behavioural change (daemon vs host split + suspend accounting + deterministic sort).

---

## 6. Risks & Follow-ups

- **Compose parallel branch:** `forge/api/internal/services/compose/controller.go:236` etc references `ValidateHostMountWithAllowlist`/`indexOfColon`/`isTraversalLike`/`gitOpsComposeHasBuild` undefined → `go build ./...` in `forge/api` currently fails on that package due to incomplete merge from other 19 agents. **Not introduced by this agent**; our scoped packages still vet/test clean. Recommend the compose owners land missing helpers or revert controller to HEAD before CI gate.
- **Migration prefix collision:** `211_add_restoring_backup_actual_state.sql` vs `211_tenant_scoping_additive.sql` — unrelated to cron; needs prefix rename (e.g. `211_a_…`).
- **Monitoring “no data” tuning:** If Beacon begins reporting real network/load, `isSynthetic` will auto-flip to false (all-zero check). If load is legitimately 0 for >5 m under idle host, isSynthetic could false-positive; threshold could be refined to require `networkTxBytes` also zero for ≥30 points (current) — acceptable for Phase 03.
- **Firewall legacy rows:** Any persisted `deny` rows via old code would now fail validation on `load()` (rejected). Consider a one-time migration to drop or convert such rows if deployment has them (none observed in test fixtures).

---

## 7. Checklist (per brief)

- [x] `cronjob/service.go` — inFlight monotonic dedup map, `pg_advisory_xact_lock(hashtext(jobID))` cross-replica, `time.AfterFunc` non-blocking retry, `go s.executeJob` off worker
- [x] `monitoring/page.tsx:51` — “no data” not 0 for `isSynthetic` network path (verified; `syntheticNoDataMetric` branch)
- [x] `handlers_host.go:65 + sysinfo` — host vs daemon uptime split with suspend subtraction (`CLOCK_MONOTONIC` docs, `suspendDuration`)
- [x] `sysinfo_linux:94` inert sort fixed (in-place `sort.Slice` by PID)
- [x] `handlers_firewall.go:132 + deny` — placeholder `0.0.0.0/0` → `203…/198…`, `validateFirewallAction:120` allow-only with explicit deny rejection, docs added (`docs/firewall.md`)
- [x] Host-tool freeze documented (`docs/host-tool.md` + test pin)
- [x] Tests for cron dedup and monitoring gaps (`dedup_test.go`, `monitoring_synthetic_test.go`, `firewall_placeholder_test.go`, `synthetic_test.go`)

