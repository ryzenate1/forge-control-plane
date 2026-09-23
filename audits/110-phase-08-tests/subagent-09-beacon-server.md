# Subagent 09 — Beacon Server / Manager / Runtime Tests (Phase 08)

**Agent:** 09/20  
**Focus:** `beacon/internal/server/server.go:805` Provider 400, `compose.go:213` shortFormHostPort fix, `manager.go:30` RestoreState  
**Date:** 2026-08-24

## Task

- Inspect `beacon/internal/server/server.go:805` Provider field 400, `compose.go:213` shortFormHostPort fix, `manager.go:30` RestoreState
- Check existing: `ls beacon/internal/server/*test.go | head -n 20`
- Create/augment: `beacon/internal/server/phantom_test.go` verify `go test ./beacon/internal/server -run TestPhantom`
- Create/augment: `beacon/internal/server/manager_restore_test.go` verify `go test ./beacon/internal/server -run TestManager_Restore`
- Run: `go test ./beacon/internal/server -count=1` and `go test ./beacon/internal/runtime -count=1`
- Report here

## Inspection

### 1. `beacon/internal/server/server.go:75-89, 805-909` — Provider field 400

`beacon/internal/server/server.go:795-828` — `create()` decodes JSON body including `Provider string json:"provider,omitempty"` at line 827.

`beacon/internal/server/server.go:841-847` — **honesty fix 110-03-17**: validates provider via `isSupportedBeaconProvider()` (lines 75-89), returns `400 unsupported provider` instead of silently mapping to docker.

`beacon/internal/server/server.go:75-89` — `isSupportedBeaconProvider()` checks `supportedBeaconProviders` allowlist; LXC/KVM only allowed when `ENABLE_EXPERIMENTAL_RUNTIMES=true/1/yes` (via `isExperimentalRuntimeEnabled()` at line 70-73). Empty provider returns true (defaults to docker). Lower-case + trim normalization ensures case-insensitive handling. Line 904-907 sets `mode` to provider or docker default in 202 response.

### 2. `beacon/internal/server/compose.go:282-315` — shortFormHostPort fix

`beacon/internal/server/compose.go:282-315` — `shortFormHostPort(entry string)` correctly:

- Strips `/tcp` `/udp` `/sctp` suffix before splitting (lines 292-294)
- `len=1` → returns `parts[0]` (single port like `"80"` was previously mishandled; this was the bug)
- `len=2` → returns `parts[0]` (host:container)
- `len=3` → returns `parts[1]` (ip:host:container)
- `len>3` → returns `parts[len-2]` (IPv6 bracketed `"[::1]:8080:80"` → `"8080"`)
- `validateComposePorts` at `compose.go:234-280` uses it, then handles range suffix `"-"` by taking low port (line 263-265), and checks privileged `<1024` rejection.

Prior tasks note `compose.go:213 shortFormHostPort fix` — actual function is at line 286; line 213 is inside `composeString`. The referenced fix is the len=1 branch now returning `parts[0]` instead of `""`. Verified working.

### 3. `beacon/internal/server/manager.go:30-37, 320-372, 544-563` — RestoreState

`beacon/internal/server/manager.go:30-37` — `ServerState.RestoreState string` + `Restoring bool` + `RunningAction` track restore lifecycle.

- `BeginRestore` (321-347): TryLock, checks `RunningAction!=""`, `Suspended`, `InstallationState=="installing"`, `Restoring` → sets `RunningAction="restore"`, `Restoring=true`, `RestoreState="restoring"`.
- `EndRestore` (351-364): clears `RunningAction=="restore"`, sets `Restoring=false`, `RestoreState="restored"` or `"failed"`.
- `IsRestoring` (367-372): `Restoring || RestoreState=="restoring" || RunningAction=="restore"`.
- `HandlePower` (544-563): rejects if `Restoring || RestoreState=="restoring"` with `"server is restoring backup"`, and also rejects via `RunningAction!=""`.
- `BeginInstall` (273-298): rejects if `Restoring || RestoreState=="restoring"` → prevents install/restore race (GH-09 P1).

## Existing Test Files

```
ls beacon/internal/server/*test.go | head -n 20 → 20 shown, 30 total test files (5201 lines total)
- audit_fixes_test.go
- backup_progress_wiring_test.go
- compose_fixes_test.go
- console_recovery_test.go
- console_test.go
- crashloop_breaker_test.go
- diagnostics_test.go
- edge_test.go
- files_test.go
- firewall_placeholder_test.go
- git_credentials_test.go
- hostfiles_confinement_test.go
- hostfiles_test.go
- install_exclusivity_test.go
- manager_restore_test.go  ← phase 03-02
- manager_test.go
- metrics_localhost_test.go
- monitoring_synthetic_test.go
- mounts_test.go
- phantom_test.go  ← phase 03-17
(+ 10 more: queue_*, runtime_handlers, sanitize_*, secure_files, server, stats_collector, upgrade)
```

## Test Verification

### `phantom_test.go` (phase 03-17) — `go test -run TestCreatePhantomProvider`

`beacon/internal/server/phantom_test.go:10-112` contains:
- `TestCreatePhantomProvider_Rejected` — lxc/kvm/LXC/KVM/unknown-phantom → 400 "unsupported provider", no `runtime.Create` call; docker/containerd/podman/firecracker/kubernetes/"" → 202, create called.
- `TestCreatePhantomProvider_AllowedWithExperimentalFlag` — ENABLE_EXPERIMENTAL_RUNTIMES=true → lxc/kvm accepted (202).
- `TestCreatePhantomProvider_ModeReflectsProvider` — mode field is docker or provider.

```
go test ./beacon/internal/server -run TestCreatePhantomProvider -count=1 -v
  TestCreatePhantomProvider_Rejected (0.04s) — PASS (11 subcases)
  TestCreatePhantomProvider_AllowedWithExperimentalFlag (0.01s) — PASS (4 subcases)
  TestCreatePhantomProvider_ModeReflectsProvider (0.01s) — PASS
  ok gamepanel/beacon/internal/server 2.164s

go test -run TestPhantom → warning: no tests to run (pattern is TestCreatePhantomProvider*, not TestPhantom*)
  Correct invocation is -run TestCreatePhantomProvider
```

No augmentation needed — existing 03-17 tests already cover Provider 400 contract thoroughly (case-insensitive, whitespace, empty, allowlist, flag gate, no side-effect on rejection, mode reflection).

### `manager_restore_test.go` (phase 03-02) — `go test -run TestManager_Restore`

`beacon/internal/server/manager_restore_test.go:15-193` contains:
- `TestManager_RestoreBlocksPowerAndInstall` — BeginRestore blocks HandlePower, BeginInstall, second BeginRestore; IsRestoring lifecycle; EndRestore cleared → power allowed again; failed restore also clears.
- `TestManager_RestoreBlocksConcurrentPowerAction` — concurrent HandlePower goroutine rejected during restore.
- `TestManager_PowerBlocksRestore` — HandlePower holding RunningAction blocks BeginRestore, then allowed after release.
- `TestManager_HandlePowerRejectsWhenRestoringFlagSet` — direct Restoring/RestoreState flag test.
- `TestManager_RestoreStateTransitions` — empty → restoring → restored → failed state machine.

```
go test ./beacon/internal/server -run TestManager_Restore -count=1 -v
  TestManager_RestoreBlocksPowerAndInstall — PASS
  TestManager_RestoreBlocksConcurrentPowerAction — PASS
  TestManager_RestoreStateTransitions — PASS (TestManager_PowerBlocksRestore & HandlePowerRejects filtered by prefix)
  ok 3.686s

go test -run "TestManager_RestoreBlocksConcurrentPowerAction|TestManager_PowerBlocksRestore"
  both PASS
```

No augmentation needed — 03-02 coverage already exercises RestoreState mutual exclusion in both directions plus state transitions.

### `compose_fixes_test.go` — shortFormHostPort

`beacon/internal/server/compose_fixes_test.go:12-57`
- `TestShortFormHostPort_Fixes` — 9 cases: len1, len1+protocol, host:container, range, ip:host:container, random host `::`, ip:range, protocol suffix, bracket ipv6 → all PASS
- `TestValidateComposePorts_RangePrivileged` — privileged range rejection, non-privileged pass, len1 privileged, random host not privileged → PASS
- `TestValidateComposePorts` backwards compat via `validateComposePorts` at `compose.go:234`.

```
go test -run TestShortFormHostPort -count=1 -v → PASS 0.00s
go test -run TestValidateComposePorts -count=1 -v → PASS
```

### Full Suite Runs

```
go test ./beacon/internal/server -count=1 → ok 3.558s–5.033s (PASS, ~45 top-level tests, all subcases pass)
  Notable passing: TestReadyIsPublicProbe, TestMetrics*, TestPowerRejectsInvalidSignal,
  TestCreateRejectsUnavailableRuntime, TestBackup*, TestSignedRequest*, TestFileAPI*,
  TestChunkedUpload*, TestStatsCollector*, TestUpgrade* plus phantom/compose/manager_restore

go test ./beacon/internal/runtime -count=1 → ok 1.838s–4.437s (PASS, 17 tests)
  RegistryAuthIsScoped, DecodeEnhancedStats*, Kubernetes*, EnvVarsFromSlice,
  ContainerPorts, BuildResource*, IsNotFound, SignalToNumber, EncodeBase64,
  DecodeDockerStats
```

## Coverage Gaps Assessed (no new tests added)

- **Provider field:** Already covers empty/docker allowlist, LXC/KVM blocked, case/whitespace normalization, unknown phantom, experimental flag on/off, no runtime side-effect, mode reflection. Gap `nil` provider (JSON omitted) is covered by `""` case (zero value). 400 body contains "unsupported provider" asserted.
- **shortFormHostPort:** Already covers all `len(parts)` branches (1,2,3,>3), protocol stripping, range extraction, privileged port interaction, random host `""` edge. One minor uncovered edge is malformed bracket missing closing bracket — falls into `len>3` path and still returns second-last, which is acceptable (validation then ignores non-numeric).
- **RestoreState:** Already covers Begin/End lifecycle, IsRestoring, concurrent power vs restore (both directions), direct flag injection, sequential restore after EndRestore, failed vs restored terminal states. Power `stop`/`restart`/`kill` paths during restore are covered by the same HandlePower guard (signal-agnostic rejection).

Existing 03-02 and 03-17 files are sufficient; augmenting would duplicate coverage. Both verified green with `-count=1 -v`.

## Verdict

**PASS** — All three focus areas have fixes landed and tests green:

| Area | File:Line | Fix | Tests | Result |
|------|-----------|-----|-------|--------|
| Provider 400 | `server.go:841-847` | LXC/KVM rejected 400 unless `ENABLE_EXPERIMENTAL_RUNTIMES` | `phantom_test.go:13-112` (3 tests, 15 subcases) | PASS 2.164s |
| shortFormHostPort | `compose.go:286-315` | len1 `"80"` now returns host port, not `""` | `compose_fixes_test.go:12-57` (2 tests, 9 subcases) | PASS |
| RestoreState | `manager.go:30-37,321-372` | BeginRestore/EndRestore/HandlePower mutual exclusion | `manager_restore_test.go:15-193` (5 tests) | PASS 3.686s |

```
go test ./beacon/internal/server -count=1  → ok
go test ./beacon/internal/runtime -count=1 → ok
```
