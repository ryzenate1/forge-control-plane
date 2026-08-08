package envaffinity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gamepanel/forge/internal/placement"
	"gamepanel/forge/internal/store"
)

// Ranking is one row of the affinity viewer's ranked shortlist.
type Ranking struct {
	NodeID  string   `json:"nodeId"`
	Score   float64  `json:"score"`
	Env     string   `json:"env,omitempty"`
	Reasons []string `json:"reasons"`
}

// ExplainResult is the "affinity viewer" payload: a readable walkthrough of
// why a node is (or is not) a candidate for a workload, using the exact
// label constraints the placement path enforces.
type ExplainResult struct {
	ID              string                 `json:"id"`
	RequestedEnv    string                 `json:"requestedEnv,omitempty"`
	NodeEnvGroups   []string               `json:"nodeEnvGroups"`
	NodeLabels      map[string]string      `json:"nodeLabels"`
	Constraints     []placement.Constraint `json:"constraints"`
	MatchedLabels   []string               `json:"matchedLabels"`
	MissingLabels   []string               `json:"missingLabels"`
	IsCandidate     bool                   `json:"isCandidate"`
	Ranking         []Ranking              `json:"ranking,omitempty"`
}

var errNodeNotFound = errors.New("node not found")

// ExplainPlacement implements the "affinity viewer". It constructs a fresh
// placement engine on its own code path (the scheduler service is not
// exposed through http.Config), enriches requests with env-derived label
// constraints, and reports the candidate node's env match, matched labels,
// missing labels and a ranked shortlist across the fleet.
func (m *EnvAffinity) ExplainPlacement(ctx context.Context, nodeID string, req domain.PlacementRequest) (ExplainResult, error) {
	result := ExplainResult{NodeID: nodeID, NodeLabels: map[string]string{}}

	enriched, err := m.EnrichPlacementRequest(ctx, req)
	if err != nil {
		return result, fmt.Errorf("enrich placement: %w", err)
	}
	result.RequestedEnv = enriched.EnvGroup
	result.Constraints = enriched.Constraints

	nodes, err := m.store.ListNodes(ctx)
	if err != nil {
		return result, err
	}
	target := store.Node{}
	found := false
	for _, node := range nodes {
		if node.ID == nodeID {
			target = node
			found = true
			break
		}
	}
	if !found {
		return result, errNodeNotFound
	}
	for _, pair := range target.Labels {
		result.NodeLabels[pair.Key] = pair.Value
	}
	groups, err := m.store.ListNodeEnvGroups(ctx)
	if err == nil {
		for _, g := range groups {
			if g.NodeID == nodeID {
				result.NodeEnvGroups = g.EnvGroups
				result.NodeLabels[EnvKey] = strings.Join(g.EnvGroups, ",")
			}
		}
	}

	switch {
	case result.RequestedEnv == "":
		// No env pin: nothing to judge; every node stays a candidate.
	case NodeSatisfiesEnv(enriched.Ctx.NodeLabels, nodeID, result.RequestedEnv):
		result.MatchedLabels = append(result.MatchedLabels,
			fmt.Sprintf("%s in {%s}", EnvKey, result.RequestedEnv))
	default:
		result.MissingLabels = append(result.MissingLabels,
			fmt.Sprintf("%s not in {%s}", EnvKey, result.RequestedEnv))
	}

	// Own engine built from the same store facts the scheduler would use.
	candidates := make([]placement.Candidate, 0, len(nodes))
	for _, node := range nodes {
		snapshot, err := m.store.NodeCapacitySnapshot(ctx, node.ID)
		if err != nil {
			continue
		}
		candidates = append(candidates, placement.Candidate{
			NodeID:          node.ID,
			RegionID:        regionOf(node),
			TotalCPU:        snapshot.TotalCPU,
			TotalMemory:     snapshot.TotalMemory,
			TotalDisk:       snapshot.TotalDisk,
			AllocatedCPU:    snapshot.AllocatedCPU,
			AllocatedMemory: snapshot.AllocatedMemory,
			AllocatedDisk:   snapshot.AllocatedDisk,
			AvailableCPU:    snapshot.AvailableCPU,
			AvailableMemory: snapshot.AvailableMemory,
			AvailableDisk:   snapshot.AvailableDisk,
			ServerCount:     snapshot.ServerCount,
			Status:          node.Status,
		})
	}
	if len(candidates) == 0 {
		return result, errors.New("no nodes have capacity snapshots")
	}

	workload := placement.WorkloadRequest{
		CPU:             ensurePositive(req.CPU, 1024),
		MemoryMB:        ensurePositive(req.MemoryMB, 2048),
		DiskMB:          ensurePositive(req.DiskMB, 10240),
		PreferredNode:   req.PreferredNode,
		RequiredNode:    req.RequiredNode,
		RegionID:        req.RegionID,
		StorageLocality: req.StorageLocality,
		Constraints:     enriched.Constraints,
		ConstraintCtx:   enriched.Ctx,
	}

	engine := placement.NewEngine(placement.NewScorer(placement.StrategyLeastLoaded),
		placement.NewConstraintChecker())
	results, err := engine.PlaceAll(ctx, candidates, workload)
	if err != nil {
		return result, err
	}
	for _, r := range results {
		row := Ranking{NodeID: r.NodeID, Score: r.Score, Reasons: r.Reasons}
		if env := envOf(enriched.Ctx.NodeLabels, r.NodeID); env != "" {
			row.Env = env
		}
		result.Ranking = append(result.Ranking, row)
	}

	result.IsCandidate = len(result.MissingLabels) == 0
	return result, nil
}

func regionOf(node store.Node) string {
	if node.RegionID != nil {
		return *node.RegionID
	}
	return ""
}

func envOf(labels map[string]map[string]string, nodeID string) string {
	node := labels[nodeID]
	if node == nil {
		return ""
	}
	return node[EnvKey]
}

func ensurePositive(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}