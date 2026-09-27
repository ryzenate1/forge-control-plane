package placement

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gamepanel/forge/internal/runtime"
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

func (e *Engine) PlaceReplicas(ctx context.Context, candidates []Candidate, req ReplicaPlacementRequest) (*ReplicaPlacementResult, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates available for replica placement")
	}

	result := &ReplicaPlacementResult{
		Placements: make([]ReplicaPlacement, 0, len(req.Replicas)),
	}

	workingCandidates := append([]Candidate(nil), candidates...)
	if req.StorageLocality != "" {
		workingCandidates = filterByStorageLocality(workingCandidates, req.StorageLocality)
		if len(workingCandidates) == 0 {
			return nil, fmt.Errorf("no candidates satisfy storage locality %q", req.StorageLocality)
		}
	}
	usedNodeCount := make(map[string]int)
	for nodeID, count := range req.ExistingNodeMap {
		usedNodeCount[nodeID] = count
	}
	if len(req.ExistingUsage) > 0 {
		// Exact accounting: subtract what existing instances actually consume.
		for index := range workingCandidates {
			if usage, ok := req.ExistingUsage[workingCandidates[index].NodeID]; ok {
				workingCandidates[index].AvailableCPU -= usage.CPU
				workingCandidates[index].AvailableMemory -= usage.MemoryMB
				workingCandidates[index].AvailableDisk -= usage.DiskMB
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
			for index := range workingCandidates {
				if workingCandidates[index].NodeID == nodeID {
					workingCandidates[index].AvailableCPU -= count * perCPU
					workingCandidates[index].AvailableMemory -= count * perMemory
					workingCandidates[index].AvailableDisk -= count * perDisk
				}
			}
		}
	}

	for _, replica := range req.Replicas {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("replica placement cancelled: %w", err)
		}
		placement, err := e.placeSingleReplica(ctx, workingCandidates, replica, req, usedNodeCount)
		if err != nil {
			result.Failures = append(result.Failures, ReplicaFailure{
				Index:  replica.Index,
				Reason: err.Error(),
			})
			continue
		}
		usedNodeCount[placement.NodeID]++
		for index := range workingCandidates {
			if workingCandidates[index].NodeID == placement.NodeID {
				workingCandidates[index].AvailableCPU -= replica.CPU
				workingCandidates[index].AvailableMemory -= replica.MemoryMB
				workingCandidates[index].AvailableDisk -= replica.DiskMB
				workingCandidates[index].AllocatedCPU += replica.CPU
				workingCandidates[index].AllocatedMemory += replica.MemoryMB
				workingCandidates[index].AllocatedDisk += replica.DiskMB
				workingCandidates[index].ServerCount++
			}
		}
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

func (e *Engine) placeSingleReplica(ctx context.Context, candidates []Candidate, replica ReplicaSpec, req ReplicaPlacementRequest, usedNodeCount map[string]int) (*ReplicaPlacement, error) {
	filtered := filterByRuntime(candidates, replica.RuntimeProvider)
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
				sp, err := e.scoreReplicaCandidate(ctx, c, replica, req, usedNodeCount)
				if err != nil {
					return nil, err
				}
				return &ReplicaPlacement{
					Index:           replica.Index,
					NodeID:          sp.NodeID,
					Score:           sp.Score,
					Reasons:         sp.Reasons,
					RuntimeProvider: replica.RuntimeProvider,
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
		sp, err := e.scoreReplicaCandidate(ctx, c, replica, req, usedNodeCount)
		if err != nil {
			continue
		}
		results = append(results, *sp)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no viable node for replica %d", replica.Index)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	selected := results[0]
	return &ReplicaPlacement{
		Index:           replica.Index,
		NodeID:          selected.NodeID,
		Score:           selected.Score,
		Reasons:         selected.Reasons,
		RuntimeProvider: replica.RuntimeProvider,
	}, nil
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
	want := normalizeLocality(requested)
	var filtered []Candidate
	for _, c := range candidates {
		got := normalizeLocality(c.StorageLocality)
		if got == "" {
			// A candidate that reports no locality is a docker-class node,
			// which the scheduler treats as local storage.
			got = "local"
		}
		if got == want {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// normalizeLocality collapses the spellings used for the same storage
// behaviour so "local_only", "local-only" and "local" compare equal.
func normalizeLocality(value string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	if s == "localonly" {
		return "local"
	}
	return s
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
			} else {
				score, reasons, _ := engine.scorer.Score(ctx, c, WorkloadRequest{
					CPU:      replica.CPU,
					MemoryMB: replica.MemoryMB,
					DiskMB:   replica.DiskMB,
				})
				bonus, bonusReasons := engine.checker.CheckSoft(c, req.Constraints, req.ConstraintCtx)
				ce.Score = score + bonus
				ce.Reasons = append(reasons, bonusReasons...)
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
