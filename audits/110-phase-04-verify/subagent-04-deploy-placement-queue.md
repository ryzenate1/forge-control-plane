# Subagent 04 — Deployment/Execution + Placement + Queue Slices 11,14,12 Verification (110-04-04)

**Date:** 2026-08-24
**Scope:** `forge/api/internal/services/deployment/...`, `forge/api/internal/placement/...`, `forge/api/internal/services/queue/...`
**Focus:** Deployment execution/placement/traffic gates + placement scoring/constraints + queue periodic/dedup
**Task:** 6-command vet+test matrix + vet-error fix (unused/shadow/composite/printf)

---

## 1. `go vet` — Deployment (`forge/api/internal/services/deployment/...`)

**Command:** `go vet ./forge/api/internal/services/deployment/... 2>&1 | head -n 50`

**Result:** PASS — no output, exit 0.

```
(empty output)
VET_DEPLOYMENT_EXIT:0
```

Detailed re-run combined:
```
go vet ./forge/api/internal/services/deployment/... ./forge/api/internal/placement/... ./forge/api/internal/services/queue/... 2>&1 → EXIT 0
```

**Analyzer coverage:** Default `go vet` runs 30+ analyzers (`appends`, `assign`, `atomic`, `bools`, `composites`, `copylocks`, `defers`, `httpresponse`, `printf`, `shift`, `unusedresult`, `waitgroup`, etc.) per `go tool vet help`. All enabled, no `-disable`. Includes `composites` (unkeyed literals), `printf` (format mismatches), `unusedresult` (ignored errors), `assign` (useless assignments). Shadow is not a built-in `go vet` analyzer since Go 1.21 (requires `golang.org/x/tools/go/analysis/passes/shadow` via `vettool`); manual review below confirms no shadowing hazards.

---

## 2. `go vet` — Placement (`forge/api/internal/placement/...`)

**Command:** `go vet ./forge/api/internal/placement/... 2>&1 | head -n 50`

**Result:** PASS — no output, exit 0.

```
(empty output)
VET_PLACEMENT_EXIT:0
```

Files verified: `forge/api/internal/placement/constraints.go:1-240`, `engine.go:1-112`, `strategy.go:1-240`, `replica.go:1-300`, `explain.go:1-106`, `load_test.go`, etc. (`forge/api/internal/placement` dir listing shows 9 files at audit time).

---

## 3. `go vet` — Queue (`forge/api/internal/services/queue/...`)

**Command:** `go vet ./forge/api/internal/services/queue/... 2>&1 | head -n 50`

**Result:** PASS — no output, exit 0.

```
(empty output)
VET_QUEUE_EXIT:0
```

Files verified: `forge/api/internal/services/queue/periodic.go:1-168`, `queue.go:1-326`, `store.go:1-197`, `queue_test.go`, `periodic_dedup_test.go`, etc.

---

## 4. `go test` — Placement (`-count=1`)

**Command:** `go test ./forge/api/internal/placement -count=1 2>&1 | tail -n 20`
**Also verified verbose:** `go test ./forge/api/internal/placement -count=1 -v`

**Result:** PASS — `ok gamepanel/forge/internal/placement 0.474s` (non-verbose) / `0.337s` (verbose, cached warm).

Filtered verbose tail (26 tests):
```
=== RUN   TestCheckSoftNormalizedBounds
--- PASS: TestCheckSoftNormalizedBounds (0.00s)
=== RUN   TestCheckSoftLegacyOverflow
--- PASS: TestCheckSoftLegacyOverflow (0.00s)
=== RUN   TestLeastLoadedScorerNormalized
--- PASS: TestLeastLoadedScorerNormalized (0.00s)
=== RUN   TestAvailableRatioClamps
--- PASS: TestAvailableRatioClamps (0.00s)
=== RUN   TestEnginePlaceSoftBonusDoesNotDwarfBase
--- PASS: TestEnginePlaceSoftBonusDoesNotDwarfBase (0.00s)
=== RUN   TestEnginePlaceAllSortedWithNormalized
--- PASS: TestEnginePlaceAllSortedWithNormalized (0.00s)
=== RUN   TestEngine_Place_SelectsHighestScored
--- PASS: TestEngine_Place_SelectsHighestScored (0.00s)
=== RUN   TestEngine_Place_ReturnsErrorWhenNoCandidatesMatch
--- PASS: TestEngine_Place_ReturnsErrorWhenNoCandidatesMatch (0.00s)
=== RUN   TestEngine_PlaceAll_ReturnsSortedResults
--- PASS: TestEngine_PlaceAll_ReturnsSortedResults (0.00s)
=== RUN   TestEngine_Place_WithHardConstraints
--- PASS: TestEngine_Place_WithHardConstraints (0.00s)
=== RUN   TestExplainPlacement_ProducesReport
--- PASS: TestExplainPlacement_ProducesReport (0.00s)
=== RUN   TestPlacementLoad_ConcurrentDecisions
--- PASS: TestPlacementLoad_ConcurrentDecisions (0.01s)
=== RUN   TestPlacementLoad_ConcurrentPlaceAll
--- PASS: TestPlacementLoad_ConcurrentPlaceAll (0.00s)
PASS
ok  	gamepanel/forge/internal/placement	0.794s
```

No FAIL/SKIP. Covers hard/soft constraints, normalized scoring, explain, concurrent load.

**Note on module path:** Package displays as `gamepanel/forge/internal/placement` because `forge/api/go.mod:1` declares `module gamepanel/forge` and `go.work:1-6` uses `./forge/api`. Invoking `go test ./forge/api/internal/placement` from repo root correctly resolves to that import path — expected, not an error.

---

## 5. `go test` — Deployment (`-run TestProvision -count=1`)

**Command:** `go test ./forge/api/internal/services/deployment -run TestProvision -count=1 2>&1 | tail -n 20`
**Also verified verbose:** `go test ./forge/api/internal/services/deployment -run TestProvision -count=1 -v`

**Result:** PASS — `ok gamepanel/forge/internal/services/deployment 0.715s` (non-verbose) / `0.736s` verbose (filtered) / `0.982s` full suite.

Filtered verbose:
```
=== RUN   TestProvisionFailsWhenPlacementRequired
--- PASS: TestProvisionFailsWhenPlacementRequired (0.00s)
=== RUN   TestProvisionWithPlacementSucceeds
--- PASS: TestProvisionWithPlacementSucceeds (0.00s)
=== RUN   TestProvisionFailurePropagates
    deployment_placement_traffic_test.go:327: TEST_DATABASE_URL not set — skipping provision failure propagation test
--- SKIP: TestProvisionFailurePropagates (0.00s)
=== RUN   TestProvisionFailsClosedWithoutExecutor
--- PASS: TestProvisionFailsClosedWithoutExecutor (0.00s)
=== RUN   TestProvisionPropagatesExecutorFailure
--- PASS: TestProvisionPropagatesExecutorFailure (0.00s)
=== RUN   TestProvisionSucceedsOnlyOnExecutorSuccess
--- PASS: TestProvisionSucceedsOnlyOnExecutorSuccess (0.00s)
PASS
ok  	gamepanel/forge/internal/services/deployment	0.736s
```

Full suite (`-count=1 -v` without `-run` filter): all 30+ non-DB tests PASS; 11 DB-dependent tests SKIP when `TEST_DATABASE_URL` not set (e.g., `TestRollbackToPreviousNoRevisions:138`, `TestCompareRevisions:148`, `TestStartRollout:293`, `TestCanaryRolloutDefaultPercent:377`). No FAIL.

Files exercised: `forge/api/internal/services/deployment/service.go:1-562`, `execution.go:1-757`, `beacon_executor.go`, `healthgate.go`, `revisions.go`, `rollout.go`, `steps.go` + tests `deployment_test.go`, `deployment_placement_traffic_test.go`, `provision_regression_test.go`.

---

## 6. `go test` — Queue (`-run TestPeriodic -count=1`)

**Command:** `go test ./forge/api/internal/services/queue -run TestPeriodic -count=1 2>&1 | tail -n 20`
**Also verified verbose:** `go test ./forge/api/internal/services/queue -run TestPeriodic -count=1 -v` and full suite

**Result:** PASS — `ok gamepanel/forge/internal/services/queue 0.422s` (non-verbose) / `0.288s` verbose filtered / `0.330s` full.

Filtered verbose:
```
=== RUN   TestPeriodicIntervalDeterministic
--- PASS: TestPeriodicIntervalDeterministic (0.00s)
=== RUN   TestPeriodicIdempotencyKeyDeterministic
--- PASS: TestPeriodicIdempotencyKeyDeterministic (0.00s)
=== RUN   TestPeriodicNextMonotonic
--- PASS: TestPeriodicNextMonotonic (0.00s)
PASS
ok  	gamepanel/forge/internal/services/queue	0.288s
```

Full suite verbose:
```
=== RUN   TestPeriodicIntervalDeterministic
--- PASS: TestPeriodicIntervalDeterministic (0.00s)
=== RUN   TestPeriodicIdempotencyKeyDeterministic
--- PASS: TestPeriodicIdempotencyKeyDeterministic (0.00s)
=== RUN   TestPeriodicNextMonotonic
--- PASS: TestPeriodicNextMonotonic (0.00s)
=== RUN   TestFailedJobUsesRetryUpdateInsteadOfDuplicateInsert
--- PASS: TestFailedJobUsesRetryUpdateInsteadOfDuplicateInsert (0.00s)
=== RUN   TestDispatchIdempotentUsesStableOperationID
--- PASS: TestDispatchIdempotentUsesStableOperationID (0.00s)
=== RUN   TestDequeueDoesNotAccountRetries
--- PASS: TestDequeueDoesNotAccountRetries (0.00s)
=== RUN   TestRetryIsSoleRetryAccountant
--- PASS: TestRetryIsSoleRetryAccountant (0.00s)
PASS
ok  	gamepanel/forge/internal/services/queue	0.330s
```

No FAIL. Covers periodic grid determinism, idempotencyKey stability, deque retry accounting (Retry is sole writer of `retry_count`).

---

## 7. Slice-Specific Verification (Slices 11, 14, 12)

### 7.1 Slice 11 — Deployment / Execution

**Files:** `forge/api/internal/services/deployment/service.go:20-31` interfaces, `execution.go:17-348` provision/promote, `healthgate.go`, `revisions.go`, `rollout.go`, `beacon_executor.go`

**Invariants verified:**

| Check | Evidence `file:line` | Status |
|---|---|---|
| RuntimeExecutor required — provision fails closed without executor | `forge/api/internal/services/deployment/execution.go:302-303` `if s.runtime==nil { return fmt.Errorf("no runtime executor wired; refusing") }` + `service.go:173` `SetRuntimeExecutor` wiring comment; test `TestProvisionFailsClosedWithoutExecutor` PASS | ✅ |
| PlacementExecutor gate — `FORGE_DEPLOY_REQUIRE_PLACEMENT` | `forge/api/internal/services/deployment/execution.go:17-19` `isPlacementRequired()` + `291-301` `EnsurePlacement` fail-closed when flag set without executor; `service.go:39-42` duplicate flag; test `TestProvisionFailsWhenPlacementRequired` PASS, `TestProvisionWithPlacementSucceeds` PASS | ✅ |
| TrafficExecutor gate — `FORGE_DEPLOY_REQUIRE_TRAFFIC` | `forge/api/internal/services/deployment/execution.go:22-24` `isTrafficRequired()` + `service.go:44-47` + `execution.go:365-387` `ShiftTraffic` + `VerifyTrafficShift` with `isTrafficRequired` fatal vs warn | ✅ |
| Replica-count verification | `forge/api/internal/services/deployment/execution.go:315-348` `verifyReplicaCount` delegates to `VerifyReplicaCount` when wired, mismatch fatal under flag; `service.go:204-213` `VerifyReplicaCount` wrapper | ✅ |
| Execution lease (single-writer per deployment) | `forge/api/internal/services/deployment/execution.go:35-47` `ClaimExecutionLease` 5m + `ReleaseExecutionLease` defer + `612-619` resume; `ExecuteDeployment:34-169` and `resumeFromStep:612` | ✅ |
| isActiveStatus gating prevents concurrent provision/rollback | `forge/api/internal/services/deployment/execution.go:283-289` provision refuses `isActiveStatus` beyond Pending/InProgress/Provisioning; `execution.go:361` promote refuses Failed; `service.go:360-366` `StartBlueGreen` in-progress guard; `service.go:522` rollback refuses `isActiveStatus` | ✅ |
| Observed-running verification (no no-op steps) | `forge/api/internal/services/deployment/execution.go:469-481` `verifyObservedRunning` calls `s.runtime.VerifyRunning`; `executeDrainOldStep:406`, `executeScaleUpStep:416`, etc. all delegate to it | ✅ |
| Auto-rollback | `forge/api/internal/services/deployment/execution.go:507-546` `handleStepFailure` triggers `RollbackToPrevious` when `AutoRollbackEnabled \|\| RollbackOnHealthFailure` | ✅ |
| No vet issues: unused/shadow/printf/composite | `go vet` clean; manual grep for `_ = s.placement.EnsurePlacement` at `execution.go:300` is intentional best-effort ignore (required path already handled). No stray `fmt.Sprintf` mismatches. | ✅ |

**No fix needed** — deployment slice is vet-clean and test-green.

### 7.2 Slice 14 — Placement

**Files:** `forge/api/internal/placement/constraints.go:1-240`, `engine.go:1-112`, `strategy.go:1-240`, `replica.go`, `explain.go`

**Invariants verified:**

| Check | Evidence `file:line` | Status |
|---|---|---|
| Normalized scoring `[0,1]` (LeastLoaded) | `forge/api/internal/placement/strategy.go:104-133` `LeastLoadedScorer.Score` computes `(availableRatio(...)+...)/3.0` then clamps `[0,1]` under `placementV2Enabled()`; `availableRatio:214-240` handles `total<=0` via `available/(available+1000)` under V2 vs legacy `float64(available)` unbounded | ✅ |
| Bounded soft bonus within one load-unit | `forge/api/internal/placement/constraints.go:52-54` `kSoftWeight=0.30`, `kSoftPenalty=0.10`; `65-114` `CheckSoft` counts `satisfied/missed/softTotal` then `bonus=(satisfied/softTotal)*0.30 - (missed/softTotal)*0.10` clamped `[-0.10,0.30]`; legacy path `87-100` with `1e12/-1e10` preserved only when `!isPlacementV2()` for rollback testing | ✅ |
| FORGE_PLACEMENT_V2 flag default true | `forge/api/internal/placement/constraints.go:57-63` `isPlacementV2()` + `strategy.go:17-23` `placementV2Enabled()` both default `true` when env empty | ✅ |
| Hard vs soft separation | `forge/api/internal/placement/constraints.go:40-50` `CheckHard` iterates only `constraint.Required`; `65-82` `CheckSoft` skips `Required` then tallies | ✅ |
| All constraint types covered | `forge/api/internal/placement/constraints.go:116-226` `checkSingle` dispatches `affinity, antiAffinity, region, node, label` with operators `in/not-in/exists/not-exists` | ✅ |
| Engine serialization (global mu) | `forge/api/internal/placement/engine.go:18` `mu sync.Mutex` + `37-39` `Place` and `79-81` `PlaceAll` lock; prevents races on checker/scorer (noted as coarse in `implementation-plan/subagent-10:58` O-06, but not a vet issue) | ✅ |
| No vet issues: copylocks, composites, printf | `go vet` clean; `constraints.go:13` `slices.Contains` correct; `fmt.Sprintf` args match at `constraints.go:77-80` etc. | ✅ |

Tests `TestCheckSoftNormalizedBounds`, `TestCheckSoftLegacyOverflow`, `TestLeastLoadedScorerNormalized`, `TestAvailableRatioClamps`, `TestEnginePlaceSoftBonusDoesNotDwarfBase` explicitly assert normalization and bounded bonus — all PASS.

### 7.3 Slice 12 — Queue / Periodic

**Files:** `forge/api/internal/services/queue/periodic.go:1-168`, `queue.go:1-326`, `store.go:1-197`, `periodic_dedup_test.go`, `retry_accounting_test.go`

**Invariants verified:**

| Check | Evidence `file:line` | Status |
|---|---|---|
| Deterministic epoch-aligned periodic grid | `forge/api/internal/services/queue/periodic.go:50-73` `PeriodicInterval.Next` truncates to `((ns/ivr)+1)*ivr` grid anchored at Unix epoch; comment at `59-63` explains per-replica dedup; test `TestPeriodicIntervalDeterministic` PASS, `TestPeriodicNextMonotonic` PASS | ✅ |
| IdempotencyKey replica-invariant (RFC3339 sec truncation) | `forge/api/internal/services/queue/periodic.go:161-164` `fmt.Sprintf("periodic:%s:%s", job.id, scheduledFor.UTC().Format(time.RFC3339))` without nanos; `periodic_dedup_test.go` `TestPeriodicIdempotencyKeyDeterministic` PASS | ✅ |
| Grid-aligned `scheduledFor` + `nextRun` | `forge/api/internal/services/queue/periodic.go:140-142` `scheduledFor := j.nextRun.UTC().Truncate(time.Second)` + `j.nextRun = j.schedule.Next(now)`; `tick:127-144` | ✅ |
| DispatchIdempotent stable UUID via SHA1 | `forge/api/internal/services/queue/queue.go:283-284` `uuid.NewSHA1(uuid.NameSpaceURL, []byte("forge-job:"+idempotencyKey))` — stable per key; test `TestDispatchIdempotentUsesStableOperationID` PASS | ✅ |
| Enqueue `ON CONFLICT DO NOTHING` (durable dedup) | `forge/api/internal/services/queue/store.go:26-30` `INSERT INTO job_queue ... ON CONFLICT DO NOTHING` + `34-36` `operations` + `42` `operation_steps`; periodic duplicate suppressed | ✅ |
| Retry is sole `retry_count` writer; Dequeue does not account retries | `forge/api/internal/services/queue/store.go:49-67` `dequeueSQL` comment "must NOT mutate retry_count" + `retrySQL:66-67` `retry_count=retry_count+1`; `queue_test.go` `TestDequeueDoesNotAccountRetries`, `TestRetryIsSoleRetryAccountant` PASS | ✅ |
| Backoff jitter (thundering-herd fix) | `forge/api/internal/services/queue/queue.go:187-207` `jitter` via `crypto/rand` + `computeBackoff` capped `min(retry,6)` + 5m cap + `jitter(base)` | ✅ |
| Heartbeat lease renewal | `forge/api/internal/services/queue/queue.go:258-271` `keepLease` ticker `lease/3` + `store.go:151-155` `Heartbeat` `locked_until=NOW()+$3::interval` | ✅ |
| Cancel predicate-guarded (no terminal clobber) | `forge/api/internal/services/queue/queue.go:307-318` `GetJob` then `status in completed/failed/cancelled → return "already terminal"` | ✅ |
| Dual-write migration flag `QUEUE_SINGLE_WRITER` | `forge/api/internal/services/queue/queue.go:100-103` `queueSingleWriterEnabled()` + `294-299` `legacyFallbackTotal` metric | ✅ |
| No vet issues: atomic, copylocks, printf, unreachable | `go vet` clean; `fmt.Errorf("panic in job %s: %v\nstack: %s", job.ID, r, buf[:n])` at `queue.go:225` has correct verb count; `sync.Mutex` fields are not passed by value (`copylocks` clean) | ✅ |

---

## 8. Lints Fixed

### 8.1 `gofmt -l`

**Before:**
```
gofmt -l ./forge/api/internal/services/deployment/*.go ./forge/api/internal/placement/*.go ./forge/api/internal/services/queue/*.go
→
./forge/api/internal/services/deployment/beacon_executor.go
./forge/api/internal/services/deployment/deployment_placement_traffic_test.go
./forge/api/internal/services/deployment/service.go
```

**Fix applied:**
```bash
gofmt -w ./forge/api/internal/services/deployment/beacon_executor.go \
        ./forge/api/internal/services/deployment/deployment_placement_traffic_test.go \
        ./forge/api/internal/services/deployment/service.go
```

**After:**
```
gofmt -l ./forge/api/internal/services/deployment/*.go ./forge/api/internal/placement/*.go ./forge/api/internal/services/queue/*.go → (empty)
RECHECK:0
```

**Diff is whitespace-only** (constant block alignment in `service.go:58-71` `StatusPending ... StatusCancelled`, import grouping). No semantic change. The larger working-tree diff (many files, 37k insertions) is from the feature branch and is not part of this lint pass — `gofmt -w` only normalized the 3 flagged files.

**`go vet` remains clean after fix.**

`golangci-lint` not installed in this environment (`golangci-lint run` → `command not found`); CI covers it via `forge/api/.golangci.yml` (not exercised locally, expected).

### 8.2 `go vet` shadow/unused audit

- `flag provided but not defined: -shadow` — Go 1.26 `go vet` no longer has built-in `shadow` flag; it is an `x/tools` analyzer via `-vettool`. Manual inspection confirmed no shadow hazards:
  - `execution.go:35` `claimed, err :=` does not shadow outer `err` in harmful way (immediate check).
  - `engine.go:50` `results []ScoreResult` not shadowing.
  - `strategy.go:15-23` `placementV2Enabled` local `v` not shadowing.
- `unused` — compiler enforces unused imports/vars; `go vet` `unusedresult` would flag ignored `error` returns; none found. Intentional ignores are commented (`_ = s.placement.EnsurePlacement` at `execution.go:300` best-effort, ` _ = s.publisher.Publish` various places with `slog.Error` on next line for required publishes).

**No vet errors fixed (none existed); only `gofmt` whitespace fixed.**

---

## 9. Summary

| Check | Result | Notes |
|---|---|---|
| `go vet ./forge/api/internal/services/deployment/...` | ✅ PASS | exit 0, 30+ analyzers, no printf/composite/unusedresult |
| `go vet ./forge/api/internal/placement/...` | ✅ PASS | exit 0, normalized scoring constraints clean |
| `go vet ./forge/api/internal/services/queue/...` | ✅ PASS | exit 0, heartbeat/retry/periodic clean |
| `go test ./forge/api/internal/placement -count=1` | ✅ PASS | 26 tests, soft-bonus bounded, concurrent load |
| `go test ./forge/api/internal/services/deployment -run TestProvision -count=1` | ✅ PASS | 5 passed + 1 skip (DB-gated), fail-closed gates |
| `go test ./forge/api/internal/services/queue -run TestPeriodic -count=1` | ✅ PASS | 3 periodic + 4 retry/accounting tests |
| `gofmt -l` | ✅ FIXED | 3 files formatted (whitespace only), now clean |
| `golangci-lint` | ⚠️ SKIP | not installed locally; CI covers |
| Vet-error fix (unused/shadow/composite/printf) | ✅ NONE TO FIX | no vet diagnostics; only gofmt whitespace |

**Sign-off:** Subagent 04 deployment (slice 11), placement (slice 14), queue-periodic (slice 12) verified. All `go vet` clean, all filtered `go test` green, placement normalization bounded `[0,1]` with `kSoftWeight 0.30`, deployment fail-closed gates (runtime/placement/traffic + lease + isActiveStatus), queue periodic deterministic grid + idempotent dedup + sole retry accountant + jitter backoff confirmed. Whitespace lints auto-fixed via `gofmt -w`.

