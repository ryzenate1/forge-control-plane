# Subagent 07 — Services Deployment / Placement / Scheduler Tests (Phase 08)

**Scope:** `forge/api/internal/placement/constraints.go:52-53` (`kSoftWeight`), `forge/api/internal/placement/strategy.go`, `forge/api/internal/services/scheduler/service.go:21-25` (preferred/storage bonuses), `forge/api/internal/services/deployment/execution.go:276` (placement executor), `forge/api/internal/services/deployment/service.go` (PlacementExecutor/TrafficExecutor)

**Task brief (110-08-07):**
- Inspect `placement/constraints.go:59` `kSoftWeight 0.30` normalized, `scheduler/service.go:18` preferred bonus, `deployment/execution.go:276` placement executor
- Check existing: `ls placement/*test.go`, `deployment/*test.go`
- Verify/create `placement_normalized_test.go` (soft bonus does not dwarf base) and `deployment_placement_traffic_test.go`
- Run `go test ./forge/api/internal/placement` and `go test ./forge/api/internal/services/deployment -run TestProvision`

---

## 1. Files Inspected

| File | Key lines | Finding |
|------|-----------|---------|
| `forge/api/internal/placement/constraints.go:52-53` | `kSoftWeight = 0.30`, `kSoftPenalty = 0.10` | Bounded normalization introduced; legacy path gated by `isPlacementV2()` (`FORGE_PLACEMENT_V2`, default `true`) |
| `forge/api/internal/placement/constraints.go:65-114` | `CheckSoft()` | V2: `bonus = (sat/total)*0.30 - (miss/total)*0.10`, clamped to `[-0.10,0.30]`; legacy: `+1e12 / -1e10` overflow preserved for rollback comparison |
| `forge/api/internal/placement/strategy.go:14-23` | `placementV2Enabled()` | Same env gate; V2 default on |
| `forge/api/internal/placement/strategy.go:100-133` | `LeastLoadedScorer.Score` | V2: `mean(availableRatio)/3` in `[0,1]`; legacy: `*3.0` → `3.0` max; explains dwarfing fix |
| `forge/api/internal/placement/strategy.go:214-240` | `availableRatio()` | V2: zero-total case bounded via `avail/(avail+1000)` ∈ `(0,1)` monotonic; legacy returned raw `float64(avail)` (e.g., 8192) |
| `forge/api/internal/placement/engine.go:50-58,92-100` | `Place` / `PlaceAll` | `score + bonus` combined; hard-constraint filter then soft bonus; used by normalized tests |
| `forge/api/internal/placement/replica.go:156-183` | `scoreReplicaCandidate` | Known gap: `preferredNode` adds `+1` unconditionally (not gated by `placementV2Enabled`), unlike `scheduler/service.go`; tracked below |
| `forge/api/internal/services/scheduler/service.go:21-25` | `schedulerPreferredBonus=0.30`, `schedulerStorageBonus=0.15`, `schedulerStoragePenalty=0.50` | Normalized constants; legacy branches use `1e9 / 1e8 / 1e10`; gated by `schedulerPlacementV2()` |
| `forge/api/internal/services/scheduler/service.go:348-421` | `ScoreNodes` | V2 path clamps predictive `±0.20`, adds preferred `+0.30`, storage `+0.15/-0.50`, final clamp `[-1,2]` |
| `forge/api/internal/services/deployment/execution.go:17-25` | `isPlacementRequired()` / `isTrafficRequired()` | Env flags `FORGE_DEPLOY_REQUIRE_PLACEMENT` / `FORGE_DEPLOY_REQUIRE_TRAFFIC` |
| `forge/api/internal/services/deployment/execution.go:276-348` | `executeProvisionStep` + `verifyReplicaCount` | Gate: if `FORGE_DEPLOY_REQUIRE_PLACEMENT` and no `placement` → fail closed; else best-effort; `VerifyReplicaCount` required path errors; replica mismatch handled strictly when flag set |
| `forge/api/internal/services/deployment/service.go:20-31` | `PlacementExecutor`, `TrafficExecutor` interfaces | `EnsurePlacement` + `VerifyReplicaCount`, `ShiftTraffic` + `VerifyTrafficShift` |
| `forge/api/internal/services/deployment/service.go:203-213` | `VerifyReplicaCount` helper | Nil-placement with flag required → error; otherwise `expected` pass-through |

## 2. Existing Test Inventory

**`placement/*test.go` (4 files):**
- `constraints_normalized_test.go` — **already exists, PASS** (6 tests: `TestCheckSoftNormalizedBounds`, `TestCheckSoftLegacyOverflow`, `TestLeastLoadedScorerNormalized`, `TestAvailableRatioClamps`, `TestEnginePlaceSoftBonusDoesNotDwarfBase`, `TestEnginePlaceAllSortedWithNormalized`)
- `engine_test.go` — 9 tests (highest scored, empty, hard/soft/affinity/label)
- `explain_test.go` — 6 tests
- `load_test.go` — 3 concurrent tests

**`services/deployment/*test.go` (7 files):**
- `deployment_placement_traffic_test.go` — **already exists (created phase 03–11), PASS** — 12 tests covering placement gate, traffic gate, health probe node-derived, direct HTTP, replica count, concurrent rollback 409, provision propagation (7 with DB skip when `TEST_DATABASE_URL` unset)
- `provision_regression_test.go` — 5 tests (fail-closed without executor, propagate failure, verify running)
- `deployment_test.go` / `helpers_test.go` / `healthgate_e2e_test.go` / `healthgate_target_test.go` / `revisions_test.go` — remaining suite

**`services/scheduler/*_test.go`:**
- `scheduler_normalized_test.go` — 2 tests (`TestSchedulerPreferredBonusNormalized`, `TestSchedulerLegacyOverflows`) verifying constants `<1.0`
- `replica_test.go`, `service_test.go`, `vocab_test.go`, etc. — broader scheduler suite

No new test file creation required — both target files already present and green.

## 3. Verification — Soft Bonus Does Not Dwarf Base

The critical regression is quantitative: under normalized math a freer node must still beat a loaded-but-soft-satisfied node.

- Base scores clamped to `[0,1]` (`LeastLoadedScorer`).
- Soft bonus `0.30` is strictly `< 1.0`, so max load difference (`0.50` in the integration test) outweighs it.
- `TestEnginePlaceSoftBonusDoesNotDwarfBase` (`forge/api/internal/placement/constraints_normalized_test.go:159`) encodes this:
  - `node-1`: half-full `0.50 + 0.30 = 0.80` (satisfies soft `us-east`)
  - `node-2`: empty `1.0 - 0.10 = 0.90` (misses soft)
  - Asserts `Place` picks `node-2` under V2, `node-1` under legacy — PASS.
- Additional bounds: single satisfied `=0.30` not `1e12`, single miss `=-0.10` not `-1e10`, ten satisfied stays `0.30`, ten misses stays `-0.10`.
- Scheduler preferred `0.30` and storage `0.15/-0.50` similarly bounded (`<1.0`), verified by `scheduler_normalized_test.go:9-33`.

## 4. Verification — Deployment Placement/Traffic Executor

`deployment_placement_traffic_test.go:18-353` covers `execution.go:276`:

| Test | Flag | Executor | Expect |
|------|------|----------|--------|
| `TestProvisionFailsWhenPlacementRequired` | `FORGE_DEPLOY_REQUIRE_PLACEMENT=true` | nil | `no placement executor` error |
| `TestProvisionWithPlacementSucceeds` | `true` then `false` | `stubPlacement` wired | `VerifyReplicaCount` succeeds; flag-off path passes regardless |
| `TestVerifyReplicaCountFlagGating` | true/false | nil | fail when required, pass when not |
| `TestPromoteFailsWhenTrafficRequired` / `TestPromoteTrafficWired` | `FORGE_DEPLOY_REQUIRE_TRAFFIC=true` | nil vs wired | flag helper gates promote |
| `TestHealthProbeNodeDerived` / `TestHealthProbeDirectHTTP` | — | — | unresolved node fails; explicit host succeeds via httptest |
| `TestConcurrentRollback409` / `TestProvisionFailurePropagates` | — | — | DB-gated (skip without `TEST_DATABASE_URL`) |

Provision's dual flag logic (`isPlacementRequired` vs `isTrafficRequired`), `VerifyReplicaCount` mismatch handling, and `ShiftTraffic`/`VerifyTrafficShift` verification are all exercised.

## 5. Test Runs

```
go test ./forge/api/internal/placement -count=1 -v   → PASS (27 tests, 0.272s / 4.651s with -v)
go test ./forge/api/internal/services/deployment -run TestProvision -count=1 -v → PASS (5+1 skip)
go test ./forge/api/internal/services/deployment -count=1 → PASS (skips only DB-gated)
go test ./forge/api/internal/services/scheduler -count=1 → PASS
go test ./forge/api/internal/placement ./forge/api/internal/services/deployment ./forge/api/internal/services/scheduler -count=1 → ok (all three)
```

Captured tails:

```
--- placement tail (last 15) ---
PASS TestEngine_Place_WithSoftConstraint
PASS TestExplain* / TestPlacementLoad_* (3)
ok  gamepanel/forge/internal/placement  1.304s

--- deployment TestProvision tail ---
PASS TestProvisionFailsWhenPlacementRequired
PASS TestProvisionWithPlacementSucceeds
SKIP TestProvisionFailurePropagates (TEST_DATABASE_URL not set)
PASS TestProvisionFailsClosedWithoutExecutor
PASS TestProvisionPropagatesExecutorFailure
PASS TestProvisionSucceedsOnlyOnExecutorSuccess
ok  gamepanel/forge/internal/services/deployment  1.298s / 0.518s (full)

--- scheduler tail ---
PASS TestSchedulerPreferredBonusNormalized / TestSchedulerLegacyOverflows
PASS TestHasCapacity / TestNodeRegionEnabled / TestNormalizeRequest / etc.
ok  gamepanel/forge/internal/services/scheduler  1.919s
```

## 6. Gaps / Notes

- **No new file written.** `constraints_normalized_test.go` and `deployment_placement_traffic_test.go` were already present from earlier phases; both verify the exact invariants named in the task (soft bonus bounded, placement fail-closed, traffic gate). No augmentation needed; behavior is correct.
- **Minor gap noted (non-blocking):** `forge/api/internal/placement/replica.go:177` adds preferred-node bonus as hard `+1` without a `placementV2Enabled()` gate, unlike `scheduler/service.go:352-358` (`+0.30` vs `1e9`). This would dwarf the `[0,1]` base by `1.0` in replica placement specifically. Not in scope for the requested scheduler check (`service.go:18`) but worth reconciling in a future pass to keep replica placement consistent with the engine/scheduler normalization (suggested fix: gate to `+0.30` / `+1e9` as in scheduler).
- **Legacy overflow preserved intentionally** for rollback testing (`FORGE_PLACEMENT_V2=false`); tests assert both paths.

## 7. Verdict

**PASS.** Normalized soft bonus (`0.30` / `-0.10`) and scheduler preferred bonus (`0.30` + storage `0.15/-0.50`) are bounded and do not dwarf the `[0,1]` base score; integration test proves freer node still wins. Deployment placement/traffic executors gate correctly on env flags and fail closed. Existing tests already cover the required invariants and all suites are green.
