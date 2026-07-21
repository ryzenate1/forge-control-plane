package placement

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type ReplicaPlacementRequest struct {
	AppID           string
	Replicas        []ReplicaSpec
	RegionID        string
	RequiredNode    string
	PreferredNode   string
	RuntimeFilter   string
	Constraints     []Constraint
	ConstraintCtx   ConstraintContext
	ExistingNodeMap map[string]int
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

	usedNodeCount := make(map[string]int)
	for nodeID, count := range req.ExistingNodeMap {
		usedNodeCount[nodeID] = count
	}

	for _, replica := range req.Replicas {
		placement, err := e.placeSingleReplica(ctx, candidates, replica, req, usedNodeCount)
		if err != nil {
			result.Failures = append(result.Failures, ReplicaFailure{
				Index:  replica.Index,
				Reason: err.Error(),
			})
			continue
		}
		usedNodeCount[placement.NodeID]++
		result.Placements = append(result.Placements, *placement)
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
				return e.buildPlacement(c, replica, req.Constraints, req.ConstraintCtx)
			}
		}
		return nil, fmt.Errorf("required node %s not found or incompatible", req.RequiredNode)
	}

	constraintFiltered, _ := e.checker.FilterByConstraints(filtered, req.Constraints, req.ConstraintCtx)
	if len(constraintFiltered) > 0 {
		filtered = constraintFiltered
	}

	var results []scoredPlacement
	for _, c := range filtered {
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
		spreadPenalty := float64(count) * 1e8
		score -= spreadPenalty
		reasons = append(reasons, fmt.Sprintf("anti-affinity spread penalty: -%.0f (%d instances)", spreadPenalty, count))
	}

	if req.PreferredNode != "" && c.NodeID == req.PreferredNode {
		score += 1e9
		reasons = append(reasons, "preferred node bonus")
	}

	return &scoredPlacement{NodeID: c.NodeID, Score: score, Reasons: reasons}, nil
}

func (e *Engine) buildPlacement(c Candidate, replica ReplicaSpec, constraints []Constraint, ctx ConstraintContext) (*ReplicaPlacement, error) {
	if err := e.checker.CheckHard(c, constraints, ctx); err != nil {
		return nil, err
	}
	score, reasons, err := e.scorer.Score(context.Background(), c, WorkloadRequest{
		CPU:      replica.CPU,
		MemoryMB: replica.MemoryMB,
		DiskMB:   replica.DiskMB,
	})
	if err != nil {
		return nil, err
	}
	bonus, bonusReasons := e.checker.CheckSoft(c, constraints, ctx)
	return &ReplicaPlacement{
		NodeID:          c.NodeID,
		Score:           score + bonus,
		Reasons:         append(reasons, bonusReasons...),
		RuntimeProvider: replica.RuntimeProvider,
	}, nil
}

func filterByRuntime(candidates []Candidate, runtimeProvider string) []Candidate {
	if runtimeProvider == "" || runtimeProvider == "docker" {
		return candidates
	}
	var filtered []Candidate
	for _, c := range candidates {
		filtered = append(filtered, c)
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
		exp := ReplicaPlacementExplanation{
			Index:     replica.Index,
			Candidates: make([]CandidateExplanation, 0),
		}
		filtered := filterByRuntime(candidates, replica.RuntimeProvider)
		for _, c := range filtered {
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
		usedNodeCount[filtered[0].NodeID]++
		explanations = append(explanations, exp)
	}
	return explanations
}

type ReplicaPlacementExplanation struct {
	Index     int                  `json:"index"`
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
