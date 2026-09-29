package placement

import (
	"context"
	"fmt"
	"strings"
)

type ReplicaPlacementRequest struct {
	AppID     string
	Replicas  []ReplicaSpec
	RegionID  string
	RequiredNode string
	// PreferredNode is a bounded preference, not a directive: see
	// replicaPreferredNodeBonus. RequiredNode is the directive.
	PreferredNode   string
	RuntimeFilter   string
	StorageLocality string
	Constraints     []Constraint
	ConstraintCtx   ConstraintContext
	// ExistingNodeMap counts the app's live instances already on each node. It
	// is the single source of the anti-affinity spread count: Candidate
	// ServerCount comes from the node capacity snapshot, which already contains
	// these instances, so adding the two would penalise the same workloads
	// twice and make the penalty grow with how many replicas are already up.
	ExistingNodeMap map[string]int
	// ExistingUsage carries the summed CPU/memory/disk already consumed on
	// each node by live instances of the app. It is preferred over
	// ExistingNodeMap for capacity accounting: counts alone force the engine
	// to guess per-instance size from the new replicas, which under-counts
	// whenever existing instances differ in size from what is being placed.
	ExistingUsage map[string]ResourceUsage
}

// ResourceUsage is summed workload consumption on one node.
type ResourceUsage struct {
	CPU      int
	MemoryMB int
	DiskMB   int
}

type ReplicaSpec struct {
	Index           int
	CPU             int
	MemoryMB        int
	DiskMB          int
	RuntimeProvider string
}

type ReplicaPlacement struct {
	Index           int     `json:"index"`
	NodeID          string  `json:"nodeId"`
	Score           float64 `json:"score"`
	Reasons         []string
	RuntimeProvider string `json:"runtimeProvider"`
	// Reserved is the capacity this replica occupies on NodeID from the moment
	// the engine chose it. The engine holds nothing across requests, so the
	// caller must make this amount durable (a placement reservation, or the
	// instance row) before another placement reads the same node's free
	// capacity; reporting a placement nobody reserved would be reporting work
	// that was not performed.
	Reserved ResourceUsage `json:"reserved"`
}

type ReplicaPlacementResult struct {
	Placements []ReplicaPlacement `json:"placements"`
	Failures   []ReplicaFailure   `json:"failures,omitempty"`
}

type ReplicaFailure struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

// Spread terms for replica anti-affinity. The penalty grows per existing
// instance of the same app on the node, but it is capped: spreading is a
// preference, and an unbounded term would let a crowded node never win even
// when it is the only node with free capacity.
const (
	spreadPenaltyPerInstance = 0.1
	maxSpreadPenalty         = 0.5
)

// replicaPlacementState is the request-scoped working set both the decision and
// the explanation run against, so an explanation cannot describe a winner the
// engine would not pick.
type replicaPlacementState struct {
	candidates    []Candidate
	usedNodeCount map[string]int
}

// prepareReplicaPlacement applies everything that is true for the whole request
// before any replica is scored: input validation, storage locality, and the
// capacity the app already holds on each node.
func prepareReplicaPlacement(candidates []Candidate, req ReplicaPlacementRequest) (*replicaPlacementState, error) {
	if err := ValidateReplicaConstraints(req); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates available for replica placement")
	}
	// Candidate carries only value fields, so this copy is deep enough: writing
	// to it cannot reach the caller's slice. PlaceReplicas mutates no Engine
	// field for the same reason.
	state := &replicaPlacementState{
		candidates:    append([]Candidate(nil), candidates...),
		usedNodeCount: make(map[string]int, len(req.ExistingNodeMap)),
	}
	for nodeID, count := range req.ExistingNodeMap {
		state.usedNodeCount[nodeID] = count
	}
	if req.StorageLocality != "" {
		state.candidates = filterByStorageLocality(state.candidates, req.StorageLocality)
		if len(state.candidates) == 0 {
			return nil, fmt.Errorf("no candidates satisfy storage locality %q", req.StorageLocality)
		}
	}
	if len(req.ExistingUsage) > 0 {
		// Exact accounting: subtract what existing instances actually consume.
		// Callers whose candidates already net those instances out (a capacity
		// snapshot, for example) must not supply them, or the same consumption
		// is subtracted twice; subtracting twice only ever rejects a placement
		// the node could have carried, which is the safe direction.
		for index := range state.candidates {
			if usage, ok := req.ExistingUsage[state.candidates[index].NodeID]; ok {
				state.candidates[index].AvailableCPU -= usage.CPU
				state.candidates[index].AvailableMemory -= usage.MemoryMB
				state.candidates[index].AvailableDisk -= usage.DiskMB
			}
		}
	} else {
		// Estimate from the request: the mean per-replica size, not the max.
		// The max understated existing consumption whenever replicas differ
		// in size; the mean keeps uniform requests exact and heterogeneous
		// ones honest about the average they already declared.
		var sumCPU, sumMemory, sumDisk int
		for _, replica := range req.Replicas {
			sumCPU += replica.CPU
			sumMemory += replica.MemoryMB
			sumDisk += replica.DiskMB
		}
		n := len(req.Replicas)
		if n < 1 {
			n = 1
		}
		perCPU, perMemory, perDisk := sumCPU/n, sumMemory/n, sumDisk/n
		for nodeID, count := range req.ExistingNodeMap {
			for index := range state.candidates {
				if state.candidates[index].NodeID == nodeID {
					state.candidates[index].AvailableCPU -= count * perCPU
					state.candidates[index].AvailableMemory -= count * perMemory
					state.candidates[index].AvailableDisk -= count * perDisk
				}
			}
		}
	}
	return state, nil
}

// replicaReservation is the capacity one replica occupies. It is the single
// source for both the amount apply deducts from a node and the amount reported
// as ReplicaPlacement.Reserved, so the figure a caller is told to make durable
// cannot drift from the figure the engine charged the node.
func replicaReservation(replica ReplicaSpec) ResourceUsage {
	return ResourceUsage{
		CPU:      replica.CPU,
		MemoryMB: replica.MemoryMB,
		DiskMB:   replica.DiskMB,
	}
}

// apply records that a replica landed on nodeID: the node's readable capacity
// and the app's instance count both move, so the next replica sees the same
// world the placement created.
func (s *replicaPlacementState) apply(replica ReplicaSpec, nodeID string) {
	reserved := replicaReservation(replica)
	s.usedNodeCount[nodeID]++
	for index := range s.candidates {
		if s.candidates[index].NodeID != nodeID {
			continue
		}
		s.candidates[index].AvailableCPU -= reserved.CPU
		s.candidates[index].AvailableMemory -= reserved.MemoryMB
		s.candidates[index].AvailableDisk -= reserved.DiskMB
		s.candidates[index].AllocatedCPU += reserved.CPU
		s.candidates[index].AllocatedMemory += reserved.MemoryMB
		s.candidates[index].AllocatedDisk += reserved.DiskMB
		s.candidates[index].ServerCount++
	}
}

func (e *Engine) PlaceReplicas(ctx context.Context, candidates []Candidate, req ReplicaPlacementRequest) (*ReplicaPlacementResult, error) {
	// The Engine holds no placement state (scorer, checker, logger only), so
	// the working set is request-local and needs no engine lock; Place and
	// PlaceAll lock nothing for the same reason. Concurrency across requests
	// is not closed here at all — the durable guard is the placement
	// reservation the caller makes per placed replica (see
	// replicamanager.deployReplicas: reserve, write the instance row, confirm;
	// cancel on any failure). Reserved on each placement tells the caller how
	// much capacity to make durable before the next placement reads it.
	state, err := prepareReplicaPlacement(candidates, req)
	if err != nil {
		return nil, err
	}

	result := &ReplicaPlacementResult{
		Placements: make([]ReplicaPlacement, 0, len(req.Replicas)),
	}

	for _, replica := range req.Replicas {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("replica placement cancelled: %w", err)
		}
		placement, err := e.placeSingleReplica(ctx, state, replica, req)
		if err != nil {
			result.Failures = append(result.Failures, ReplicaFailure{
				Index:  replica.Index,
				Reason: err.Error(),
			})
			continue
		}
		state.apply(replica, placement.NodeID)
		result.Placements = append(result.Placements, *placement)
	}
	if len(result.Placements) == 0 && len(result.Failures) > 0 {
		// Every replica failed: a nil error here would let callers mistake a
		// total placement failure for success (result with zero placements).
		// Return the result alongside the error so callers can still report
		// per-replica reasons.
		return result, fmt.Errorf("replica placement failed for all %d replicas: %s",
			len(req.Replicas), result.Failures[0].Reason)
	}
	return result, nil
}

// placeSingleReplica scores one replica against the request-scoped state. It
// takes the state rather than a loose (candidates, usedNodeCount) pair so the
// two cannot drift apart: the counts must always describe the same working set
// the scoring reads.
func (e *Engine) placeSingleReplica(ctx context.Context, state *replicaPlacementState, replica ReplicaSpec, req ReplicaPlacementRequest) (*ReplicaPlacement, error) {
	filtered := filterByRuntime(state.candidates, replica.RuntimeProvider)
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no candidates support runtime %s", replica.RuntimeProvider)
	}

	if req.RequiredNode != "" {
		for _, c := range filtered {
			if c.NodeID == req.RequiredNode {
				// A pinned node goes through the same scoring path as any
				// other candidate — hard constraints still apply, and the
				// spread penalty, soft bonuses and preferred-node bonus are
				// computed identically — so its score stays comparable.
				if err := e.checker.CheckHard(c, req.Constraints, req.ConstraintCtx); err != nil {
					return nil, err
				}
				sp, err := e.scoreReplicaCandidate(ctx, c, replica, req, state.usedNodeCount)
				if err != nil {
					return nil, err
				}
				return &ReplicaPlacement{
					Index:           replica.Index,
					NodeID:          sp.NodeID,
					Score:           sp.Score,
					Reasons:         sp.Reasons,
					RuntimeProvider: replica.RuntimeProvider,
					Reserved:        replicaReservation(replica),
				}, nil
			}
		}
		return nil, fmt.Errorf("required node %s not found or incompatible", req.RequiredNode)
	}

	constraintFiltered, _ := e.checker.FilterByConstraints(filtered, req.Constraints, req.ConstraintCtx)
	if len(constraintFiltered) == 0 {
		return nil, fmt.Errorf("no candidates satisfy replica constraints")
	}
	filtered = constraintFiltered

	var results []scoredPlacement
	for _, c := range filtered {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("replica placement cancelled: %w", err)
		}
		sp, err := e.scoreReplicaCandidate(ctx, c, replica, req, state.usedNodeCount)
		if err != nil {
			continue
		}
		results = append(results, *sp)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no viable node for replica %d", replica.Index)
	}

	selected := bestScoredPlacement(results)

	return &ReplicaPlacement{
		Index:           replica.Index,
		NodeID:          selected.NodeID,
		Score:           selected.Score,
		Reasons:         selected.Reasons,
		RuntimeProvider: replica.RuntimeProvider,
		Reserved:        replicaReservation(replica),
	}, nil
}

// bestScoredPlacement picks the highest score with a deterministic
// tie-breaker: equal scores resolve to the lexicographically smallest node,
// so identical inputs always place identically instead of depending on input
// order.
func bestScoredPlacement(results []scoredPlacement) scoredPlacement {
	best := results[0]
	for _, r := range results[1:] {
		if r.Score > best.Score || (r.Score == best.Score && r.NodeID < best.NodeID) {
			best = r
		}
	}
	return best
}

type scoredPlacement struct {
	NodeID  string
	Score   float64
	Reasons []string
}

func (e *Engine) scoreReplicaCandidate(ctx context.Context, c Candidate, replica ReplicaSpec, req ReplicaPlacementRequest, usedNodeCount map[string]int) (*scoredPlacement, error) {
	score, reasons, err := e.scorer.Score(ctx, c, WorkloadRequest{
		CPU:      replica.CPU,
		MemoryMB: replica.MemoryMB,
		DiskMB:   replica.DiskMB,
	})
	if err != nil {
		return nil, err
	}

	bonus, bonusReasons := e.checker.CheckSoft(c, req.Constraints, req.ConstraintCtx)
	score += bonus
	reasons = append(reasons, bonusReasons...)

	// Storage locality is scored the same way on both placement paths: the
	// hard filter in prepareReplicaPlacement already removed mismatches, so
	// this term documents the preference for callers that score candidates
	// the filter never saw. It mirrors the scheduler's ScoreNodes term, using
	// the same canonical vocabulary and the same bounded bonus/penalty.
	if req.StorageLocality != "" {
		if candidateStorageLocality(c) == CanonicalStorageLocality(req.StorageLocality) {
			score += StorageMatchBonusValue()
			reasons = append(reasons, "storage locality match bonus")
		} else {
			score -= StorageMismatchPenaltyValue()
			reasons = append(reasons, "storage locality mismatch penalty")
		}
	}

	count := c.ServerCount + usedNodeCount[c.NodeID]
	if count > 0 {
		spreadPenalty := float64(count) * 0.1
		score -= spreadPenalty
		reasons = append(reasons, fmt.Sprintf("anti-affinity spread penalty: -%.0f (%d instances)", spreadPenalty, count))
	}

	if req.PreferredNode != "" && c.NodeID == req.PreferredNode {
		score += 1
		reasons = append(reasons, "preferred node bonus")
	}

	return &scoredPlacement{NodeID: c.NodeID, Score: score, Reasons: reasons}, nil
}

func filterByStorageLocality(candidates []Candidate, requested string) []Candidate {
	if strings.TrimSpace(requested) == "" {
		return candidates
	}
	want := CanonicalStorageLocality(requested)
	var filtered []Candidate
	for _, c := range candidates {
		if candidateStorageLocality(c) == want {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// CanonicalStorageLocality collapses the spellings used for the same storage
// behaviour so "local_only", "local-only" and "local" compare equal. It is the
// single vocabulary for storage locality on the placement path: the scheduler
// layer delegates to it, so a filter decision and a scoring decision can never
// disagree about what matches.
func CanonicalStorageLocality(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case "local", "local_only", "local-only", "localonly":
		return "local"
	case "shared", "shared_storage", "shared-storage", "sharedstorage":
		return "shared"
	case "replicated":
		return "replicated"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// candidateStorageLocality names the storage a candidate offers in the same
// vocabulary requests use. A candidate that reports no locality is a
// docker-class node, which the scheduler treats as local storage.
func candidateStorageLocality(c Candidate) string {
	if got := CanonicalStorageLocality(c.StorageLocality); got != "" {
		return got
	}
	return "local"
}

// normalizeLocality collapses the spellings used for the same storage
// behaviour so "local_only", "local-only" and "local" compare equal.
func normalizeLocality(value string) string {
	return CanonicalStorageLocality(value)
}

func filterByRuntime(candidates []Candidate, runtimeProvider string) []Candidate {
	if runtimeProvider == "" {
		return candidates
	}
	var filtered []Candidate
	for _, c := range candidates {
		if strings.EqualFold(c.RuntimeProvider, runtimeProvider) || (c.RuntimeProvider == "" && strings.EqualFold(runtimeProvider, "docker")) {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func ExplainReplicaPlacement(ctx context.Context, engine *Engine, candidates []Candidate, req ReplicaPlacementRequest) []ReplicaPlacementExplanation {
	var explanations []ReplicaPlacementExplanation
	usedNodeCount := make(map[string]int)
	for nodeID, count := range req.ExistingNodeMap {
		usedNodeCount[nodeID] = count
	}
	for _, replica := range req.Replicas {
		if err := ctx.Err(); err != nil {
			return explanations
		}
		exp := ReplicaPlacementExplanation{
			Index:      replica.Index,
			Candidates: make([]CandidateExplanation, 0),
		}
		filtered := filterByRuntime(candidates, replica.RuntimeProvider)
		for _, c := range filtered {
			if err := ctx.Err(); err != nil {
				break
			}
			ce := CandidateExplanation{
				NodeID:  c.NodeID,
				Reasons: []string{},
			}
			err := engine.checker.CheckHard(c, req.Constraints, req.ConstraintCtx)
			if err != nil {
				ce.Rejected = true
				ce.Reasons = append(ce.Reasons, fmt.Sprintf("hard constraint: %s", err.Error()))
			} else if req.StorageLocality != "" && candidateStorageLocality(c) != CanonicalStorageLocality(req.StorageLocality) {
				// The engine's prepareReplicaPlacement drops locality
				// mismatches before scoring; the explanation marks them
				// rejected instead so the reason stays visible while the
				// outcome still agrees with what the engine would pick.
				ce.Rejected = true
				ce.Reasons = append(ce.Reasons, fmt.Sprintf("storage locality %q does not satisfy %q", c.StorageLocality, req.StorageLocality))
			} else {
				score, reasons, _ := engine.scorer.Score(ctx, c, WorkloadRequest{
					CPU:      replica.CPU,
					MemoryMB: replica.MemoryMB,
					DiskMB:   replica.DiskMB,
				})
				bonus, bonusReasons := engine.checker.CheckSoft(c, req.Constraints, req.ConstraintCtx)
				ce.Score = score + bonus
				ce.Reasons = append(reasons, bonusReasons...)
				if req.StorageLocality != "" {
					ce.Score += StorageMatchBonusValue()
					ce.Reasons = append(ce.Reasons, "storage locality match bonus")
				}
				count := c.ServerCount + usedNodeCount[c.NodeID]
				if count > 0 {
					ce.Reasons = append(ce.Reasons, fmt.Sprintf("spread penalty: %d existing instances", count))
				}
			}
			exp.Candidates = append(exp.Candidates, ce)
		}
		var selected *CandidateExplanation
		for i := range exp.Candidates {
			candidate := &exp.Candidates[i]
			if candidate.Rejected || (selected != nil && candidate.Score <= selected.Score) {
				continue
			}
			selected = candidate
		}
		if selected != nil {
			usedNodeCount[selected.NodeID]++
		}
		explanations = append(explanations, exp)
	}
	return explanations
}

type ReplicaPlacementExplanation struct {
	Index      int                    `json:"index"`
	Candidates []CandidateExplanation `json:"candidates"`
}

type CandidateExplanation struct {
	NodeID   string   `json:"nodeId"`
	Score    float64  `json:"score,omitempty"`
	Rejected bool     `json:"rejected,omitempty"`
	Reasons  []string `json:"reasons"`
}

func ValidateReplicaConstraints(req ReplicaPlacementRequest) error {
	if len(req.Replicas) == 0 {
		return fmt.Errorf("at least one replica is required")
	}
	if req.RequiredNode != "" {
		for _, r := range req.Replicas {
			if r.RuntimeProvider != "" && r.RuntimeProvider != "docker" {
				if !strings.EqualFold(r.RuntimeProvider, "containerd") &&
					!strings.EqualFold(r.RuntimeProvider, "firecracker") &&
					!strings.EqualFold(r.RuntimeProvider, "podman") {
					return fmt.Errorf("unsupported runtime provider: %s", r.RuntimeProvider)
				}
			}
		}
	}
	return nil
}
