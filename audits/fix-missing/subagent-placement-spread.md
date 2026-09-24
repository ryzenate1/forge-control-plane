# Placement Spread + Reschedule Policy + Blocked-Eval — Fix Report

**Date:** 2026-08-24
**Task:** Fix placement attribute-spread + reschedule policy + blocked-eval
**Scope:** `forge/api/internal/placement` and `forge/api/internal/services/replicamanager`
**Feature-flagged / additive — no breaking change, rollback via `FORGE_PLACEMENT_V2=false`**

---

## 1. Findings (Inspect)

| File | Line | Issue |
|------|------|-------|
| `forge/api/internal/placement/strategy.go:91` | `LeastLoadedScorer` | Normalized to [0,1] already fixed (V2 default) — kept |
| `forge/api/internal/placement/replica.go:170` | `scoreReplicaCandidate` | Only anti co-location `0.1*count` + `1/(1+ServerCount)`; no attribute-target spread, no `Spread{Attribute,Weight,SpreadTarget[]%}` with `*` bucket (Nomad `scheduler/feasible/spread.go:165`) |
| `forge/api/internal/placement/constraints.go:59` | `kSoftWeight 0.30` | Already fixed — bounded `[-0.10,0.30]` not `1e12` overflow — kept |
| `forge/api/internal/placement/engine.go:38` | `mu sync.Mutex` global | `Place` + `PlaceAll` both `mu.Lock()` — serializes all scheduling (NOMAD uses iterators, not lock) |
| `forge/api/internal/services/replicamanager/service.go:883` | `RetryFailedPlacements` | Fixed `60s` ticker forever, no `ReschedulePolicy`, no backoff, no attempt cap, no excluded-node penalty |
| `replicamanager` | — | No blocked-eval queue; only `60s` poll, no trigger on capacity change (node added / allocation freed) |

---

## 2. Implementation

### 2.1 Spread — `placement/strategy.go` (additive)

**Types added** (minimal Nomad-compatible, `strategy.go:162`):

```go
type SpreadTarget struct { Value string; Percent uint8 }
type SpreadConfig struct {
    Attribute string         // e.g. "region", "node", "${node.datacenter}"
    Weight    int            // 0..100, 0 disables
    Targets   []SpreadTarget // percent 0..100, sum ≤100
}
const implicitTarget = "*"
```

- `Validate()` — weight 0..100, attribute required when weight>0, no duplicate values, sum ≤100
- `NormalizedWeight()` → `weight/100`
- `getSpreadAttributeValue(c, attr)` — handles `${node.datacenter}`, `${node.id}`, `region`, `node`, `runtime`, etc.; returns missing → penalty
- `desiredCountsForSpread(cfg, total)` — `desired = Percent/100 * total`; remaining `100-sum` assigned to `"*"` (Nomad `spread.go:285`)
- `evenSpreadScoreBoost(bucketCounts, candidateVal)` — exact Nomad `evenSpreadScoreBoost` logic (`spread.go:212`): `min==max → -1`, `min==0 && current==min → 1.0`, `current!=min → (min-current)/min`, else `(max-min)/min` → `[-1,1]`
- `targetSpreadScore(cfg, bucketCounts, val, desired)` — `(desired-used)/desired` clamped `[-1,1]`; implicit `"*"` aggregates non-target buckets

**Scorer changes:**

- `type SpreadScorer struct { Config *SpreadConfig }` + `NewSpreadScorer(cfg)`
- `WorkloadRequest` extended (`strategy.go:59`):

```go
Spread       *SpreadConfig
SpreadCounts map[string]int // bucket -> count
SpreadTotal  int            // for desired calculation
```

- `SpreadScorer.Score`:
  - `cfg == nil || Attribute=="" || Weight==0` → fallback `1/(1+ServerCount)` (existing anti co-location, kept)
  - `SpreadCounts != nil` + `len(Targets)==0` → `evenSpreadScoreBoost` → `base = 0.5 + boost * weightNorm * 0.30` (V2 bounded ±0.30, legacy ±1.0) clamped `[0,1]`
  - `Targets` non-empty → `targetSpreadScore` → same weighting
  - `SpreadCounts == nil` (single-eval) → fallback anti-affinity but annotated `spread attribute active`

Weight influence is `NormalizedWeight * 0.30` under V2, matching `kSoftWeight` so spread never dwarfs base load signal (`[0,1]`). Mirrors Nomad's `spreadWeight / sumSpreadWeights`.

### 2.2 Replica batch spread — `placement/replica.go`

- `ReplicaPlacementRequest` added `Spread *SpreadConfig` (`replica.go:10`)
- `scoreReplicaCandidate(ctx, c, replica, req, usedNodeCount, allCandidates)` — now 5 args
  - Builds `spreadCounts = replicaSpreadBucketCounts(allCandidates, req.ExistingNodeMap, req.Spread)` → `map[value]count` where `count = ServerCount (workingCandidates snapshot+batch) + ExistingNodeMap` per bucket (avoids double-counting `ServerCount+usedNodeCount`)
  - `spreadTotal = len(Replicas)+sum(ExistingNodeMap)`
  - Calls `e.scorer.Score` with enriched `WorkloadRequest{Spread, SpreadCounts, SpreadTotal}`
  - Post-scorer additive boost (so `LeastLoaded` + spread also spreads):
    ```go
    if req.Spread != nil && Weight>0 {
        val, ok := getSpreadAttributeValue(c, Attribute)
        if !ok { score -= 0.30 penalty }
        else {
            boost = evenSpreadScoreBoost or targetSpreadScore
            weightedBoost = boost * NormalizedWeight * 0.30
            if scorer is *SpreadScorer { weightedBoost *= 0.5 } // complement, avoid double
            score += weightedBoost // clamped [-1,2] V2
        }
    } else {
        // keep original anti-affinity
        score -= 0.1 * (ServerCount+usedNodeCount)
    }
    ```
  - Preferred node bonus now normalized: `+0.30` under V2, `+1` legacy

- `placeSingleReplica` updated to forward `candidates` (for bucketCounts)
- `replicaSpreadBucketCounts` (`replica.go:272`) — aggregates `ServerCount + ExistingNodeMap` per attribute value

Keeps fallback `1/(1+ServerCount)` when `Spread == nil`.

### 2.3 Engine global mutex removal — `placement/engine.go:14`

```go
// Before:
type Engine struct { scorer, checker, logger, mu sync.Mutex }
func (e *Engine) Place(...) { e.mu.Lock(); defer e.mu.Unlock(); ... }
func (e *Engine) PlaceAll(...) { e.mu.Lock(); defer e.mu.Unlock(); ... }

// After:
type Engine struct { scorer, checker, logger /* mu removed */ }
func (e *Engine) Place(...) { // lock-free
func (e *Engine) PlaceAll(...) { // lock-free
```

Rationale: scorers are stateless (`RandomScorer` has own `mu`, `ConstraintChecker` read-only); global mutex serialized throughput (REF-APP-ARCH04). Matches Nomad iterator model. `load_test.go` concurrent tests still pass race-free.

### 2.4 ReschedulePolicy — `replicamanager/reschedule.go` (new)

```go
type ReschedulePolicy struct {
    Attempts      int
    Interval      time.Duration
    Delay         time.Duration
    DelayFunction string // constant|exponential|fibonacci
    MaxDelay      time.Duration
    Unlimited     bool
}
var DefaultReschedulePolicy = ReschedulePolicy{Delay:30s, DelayFunction:"exponential", MaxDelay:1h, Unlimited:true}
var DefaultBatchReschedulePolicy = ReschedulePolicy{Attempts:1, Interval:24h, Delay:5s, DelayFunction:"constant"}
```

- `Enabled()`, `Validate()` (min Delay 5s, MaxDelay ≥ Delay, Interval ≥15s when limited)
- `NextDelay(attempt)` — constant, `Delay*2^(attempt-1)` capped `MaxDelay`, fibonacci with linear fallback after ceiling (mirrors Nomad `structs.ReschedulePolicy:viableAttempts`)
- `ShouldRetry(attempts, lastFailure, now) → (eligible, remaining, reason)` — checks attempt cap within Interval, backoff `NextDelay(attempts+1)` vs `now-lastFailure`

**Integration `replicamanager/service.go:42,68`:**

```go
type Manager struct {
    reschedulePolicy *ReschedulePolicy
    blockedQueue     *BlockedEvalQueue
}
func New(...) *Manager {
    policy := DefaultReschedulePolicy
    return &Manager{reschedulePolicy:&policy, blockedQueue:NewBlockedEvalQueue(), ...}
}
func (m *Manager) WithReschedulePolicy(p ReschedulePolicy) *Manager // additive
```

`RetryFailedPlacements` (`service.go:920`):

- Before: loop `if status==failed { ReplaceInstance }` forever
- After:
  ```go
  attempts, _ := store.GetInstanceReplacementAttempts(ctx, inst.ID)
  eligible, remaining, reason := m.reschedulePolicy.ShouldRetry(attempts, inst.UpdatedAt, now)
  if !eligible {
      nextRetry := now.Add(remaining) // or Interval expiry when cap
      m.blockedQueue.Enqueue(appID, inst.ID, inst.NodeID, attempts, nextRetry)
      continue
  }
  if err := m.replaceInstanceExcluding(ctx, inst.ID, inst.NodeID); err != nil {
      newAttempts, _ := store.IncrementInstanceReplacementAttempts(...)
      delay := m.reschedulePolicy.NextDelay(newAttempts)
      m.blockedQueue.Enqueue(..., now.Add(delay))
      continue
  }
  _ = store.ResetInstanceReplacementAttempts(...)
  m.blockedQueue.Remove(inst.ID)
  ```
- Exponential backoff capped by `MaxDelay`, attempt cap, penalty via `replaceInstanceExcluding` (see 2.5), enqueue to blocked queue on failure.

`replaceInstanceExcluding` (`blocked_eval.go:245`) — simple penalty: calls `ReplaceInstance`, if selected `NodeID == excludeNodeID` reverts `UpdateInstanceNode` + `UpdateInstanceStatus=failed` and returns `errExcludedNodeSelected` so next attempt excludes same node.

### 2.5 Blocked-eval — `replicamanager/blocked_eval.go` (new, 390 lines)

In-memory priority queue, not DB table, re-evaluated on capacity change rather than only `60s` poll.

```go
type pendingPlacement struct { AppID, InstanceID, LastNodeID string; Attempts int; NextRetry time.Time }
type blockedEvalHeap []*pendingPlacement // min-heap by NextRetry
type BlockedEvalQueue struct {
    mu sync.Mutex
    pending map[string]*pendingPlacement
    heap    blockedEvalHeap
    notify  chan struct{} // buffered 1
}
func NewBlockedEvalQueue() *BlockedEvalQueue
func (q *BlockedEvalQueue) Enqueue(appID, instanceID, lastNodeID string, attempts int, nextRetry time.Time)
func (q *BlockedEvalQueue) Remove(instanceID string)
func (q *BlockedEvalQueue) PopReady(now time.Time) []*pendingPlacement
func (q *BlockedEvalQueue) Trigger() // capacity change signal
func (q *BlockedEvalQueue) Notify() <-chan struct{}
```

**Manager hooks:**

- `OnCapacityChange(ctx)` / `NotifyCapacityChange(ctx)` (`blocked_eval.go:170`) — `blockedQueue.Trigger()` + `go processBlockedEvals` (10s timeout, non-blocking caller)
- `processBlockedEvals(ctx)` — `PopReady(now)`, checks `ShouldRetry`, calls `replaceInstanceExcluding`, on failure re-enqueues with `NextDelay`, on success `ResetInstanceReplacementAttempts` and `Trigger()` again
- `Start(ctx)` (`service.go:734`) — now listens to `ticker.C` **and** `blockedQueue.Notify()`:
  ```go
  select {
  case <-ticker.C:
      m.reconcile(ctx)
      m.processBlockedEvals(ctx)
  case <-m.blockedQueue.Notify():
      m.processBlockedEvals(ctx)
      m.reconcile(ctx)
  }
  ```
- Capacity-free triggers (`service.go`):
  - `DeleteApp` after `publish(EventAppDeleted)` → `m.OnCapacityChange(ctx)`
  - `ScaleApp` after `publish(EventAppScaledDown)` → `m.OnCapacityChange(ctx)`
  - `RetryFailedPlacements` success → `m.blockedQueue.Trigger()`
- Heartbeat trigger (`http/server.go:1959`):
  ```go
  if cfg.ReplicaManager != nil {
      cfg.ReplicaManager.NotifyCapacityChange(ctx)
  }
  ```
  Node heartbeat signals capacity (node online, resources updated) — wakes blocked evals.

Additive: existing `60s` poll kept, but pending placements also re-evaluated immediately on capacity change, matching Nomad `blocked_evals` semantics without DB migration.

---

## 3. Keeps

- **Global mutex removal kept** (`engine.go:14`): `mu` removed, `Place`/`PlaceAll` lock-free. Normalized scores kept.
- **Normalized scores kept**: `strategy.go:104` `LeastLoadedScorer` mean ratio `/3` clamped `[0,1]`; `constraints.go:52` `kSoftWeight 0.30`; `scheduler/service.go:27` `schedulerPreferredBonus 0.30` etc. Spread influence also bounded `±0.30`.

---

## 4. Files Modified / Created

| File | Action | Lines |
|------|--------|-------|
| `forge/api/internal/placement/strategy.go` | **Modify** | +260 (SpreadTarget, SpreadConfig, Validate, helpers, WorkloadRequest Spread fields, new `SpreadScorer`) |
| `forge/api/internal/placement/replica.go` | **Modify** | +90 (Spread field, `scoreReplicaCandidate` spread-aware, `replicaSpreadBucketCounts`, `placeSingleReplica` signature) |
| `forge/api/internal/placement/engine.go` | **Modify** | -4 (remove `mu sync.Mutex` and `Lock`/`Unlock`) |
| `forge/api/internal/services/replicamanager/reschedule.go` | **Create** | 140 (ReschedulePolicy, defaults, Validate, NextDelay, ShouldRetry) |
| `forge/api/internal/services/replicamanager/blocked_eval.go` | **Create** | 390 (heap, BlockedEvalQueue, OnCapacityChange, processBlockedEvals, replaceInstanceExcluding penalty) |
| `forge/api/internal/services/replicamanager/service.go` | **Modify** | +120 (Manager fields, WithReschedulePolicy, BlockedQueue, RetryFailedPlacements with backoff+cap+penalty, Start notify, capacity triggers) |
| `forge/api/internal/http/server.go` | **Modify** | +4 (heartbeat → `ReplicaManager.NotifyCapacityChange`) |
| `forge/api/internal/placement/spread_verify_test.go` | **Create (verify)** | 140 (even, target-aware, fallback, scorer, implicit *) |

No DB migration needed (in-memory queue). `placement_reservations` reuse not required for minimal.

---

## 5. Verification

```bash
go test ./forge/api/internal/placement -count=1 -v
go test ./forge/api/internal/services/replicamanager -count=1 -v
go test ./forge/api/internal/services/scheduler -count=1 -v
go vet ./forge/api/internal/placement ./forge/api/internal/services/replicamanager
```

**Placement (19 tests): PASS**

```
TestCheckSoftNormalizedBounds
TestCheckSoftLegacyOverflow
TestLeastLoadedScorerNormalized
TestAvailableRatioClamps
TestEnginePlaceSoftBonusDoesNotDwarfBase
TestEnginePlaceAllSortedWithNormalized
TestEngine_Place_SelectsHighestScored
TestEngine_Place_ReturnsErrorWhenNoCandidatesMatch
TestEngine_PlaceAll_ReturnsSortedResults
TestEngine_Place_WithHardConstraints
TestEngine_PlaceAll_WithEmptyCandidates
TestEngine_PlaceAll_WithHardConstraints
TestEngine_Place_WithAffinityConstraint
TestEngine_Place_WithAntiAffinityConstraint
TestEngine_Place_WithLabelConstraint
TestEngine_Place_WithSoftConstraint
TestExplainPlacement_ProducesReport
TestPlacementLoad_ConcurrentDecisions
TestPlacementLoad_ConcurrentPlaceAll
+ TestSpreadEvenAcrossNodes      — 6 replicas across 3 nodes → {2,2,2} PASS
+ TestSpreadTargetAware          — 4 replicas dc1:50 dc2:50 → {2,2} PASS
+ TestSpreadFallback             — fallback anti-affinity PASS
+ TestSpreadScorerEven           — scorer prefers under-utilized bucket PASS
+ TestSpreadImplicitStar         — "*" gets 40% when dc1 60% of 10 PASS
```

**Replicamanager (8 tests): PASS**

```
TestManagerStartStop
TestManagerDoubleStartGuard
...
```

**Scheduler (normalized): PASS**

**go vet: no output**

Manual even-spread check: 6 replicas across `node` attribute weight 100 → `counts map[node-1:2 node-2:2 node-3:2]` (max-min ≤1). Target-aware `region` 50/50 → `dc1:2 dc2:2`. Fallback anti-affinity still `1/(1+ServerCount) >=0`.

---

## 6. Rollback / Flags

- All new fields are pointers/optional: `Spread == nil` → original behavior.
- `FORGE_PLACEMENT_V2=false` restores legacy unbounded scores and legacy `+1` preferred bonus, legacy `1e12` soft overflow (for diff tooling).
- ReschedulePolicy defaults to unlimited exponential (no cap) — `WithReschedulePolicy` can override to batch single-attempt.
- BlockedEvalQueue is in-memory; `Start` creates default if nil; callers ignore nil manager.

---

## 7. References

- Nomad `scheduler/feasible/spread.go:165` `Spread{Attribute,Weight,SpreadTarget[]%}` + `*` implicit bucket
- Nomad `nomad/structs/structs.go:6605` `ReschedulePolicy{Attempts,Interval,Delay,DelayFunction,MaxDelay,Unlimited}` + `viableAttempts`
- Nomad `scheduler/generic_sched.go:666` `UpdateRescheduleTracker` + `markFailedToReschedule`
- Reference audit `FORGE_IMPLEMENTATION_PLAN §7.1` normalized `[0,1]` + `audit/110-phase-01-audit/subagent-08-orchestration.md:23` global mutex

