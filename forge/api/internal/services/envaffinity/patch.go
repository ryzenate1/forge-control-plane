package envaffinity

import (
	"context"
	"fmt"
	"sort"

	"gamepanel/forge/internal/services/scheduler"
)

// PatchResult summarizes a PatchPlacementConstraints run.
type PatchResult struct {
	ServersPinned   int      `json:"serversPinned"`
	EnvsMapped      []string `json:"envsMapped"`
	RulesRegistered int      `json:"rulesRegistered"`
}

type envNode struct {
	nodeID string
	load   int
}

// PatchPlacementConstraints is invoked from the phase 6 registrar. It
// re-reads servers.env_affinity and nodes.env_groups from the store and
// re-syncs label-selector affinity rules into the shared PredictiveScorer
// (exposed via http.Config) so later placements of a pinned server prefer
// the least-utilized node of its environment group. The sync is idempotent:
// rules carry the stable id "envaffinity-<serverID>" and are replaced, never
// duplicated.
func (m *EnvAffinity) PatchPlacementConstraints(ctx context.Context) (PatchResult, error) {
	result := PatchResult{}
	if m == nil || m.store == nil || m.scorer == nil {
		return result, fmt.Errorf("env affinity requires store and predictive scorer")
	}

	affinities, err := m.store.ListServerEnvAffinities(ctx)
	if err != nil {
		return result, err
	}

	// env -> least loaded node serving that env group.
	preferred := map[string]envNode{}
	groups, err := m.store.ListNodeEnvGroups(ctx)
	if err != nil {
		return result, err
	}
	for _, g := range groups {
		snapshot, err := m.store.NodeCapacitySnapshot(ctx, g.NodeID)
		if err != nil {
			continue
		}
		load := snapshot.AllocatedCPU + snapshot.AllocatedMemory + snapshot.AllocatedDisk
		for _, env := range g.EnvGroups {
			current, ok := preferred[env]
			if !ok || load < current.load {
				preferred[env] = envNode{nodeID: g.NodeID, load: load}
			}
		}
	}

	envs := make([]string, 0, len(preferred))
	for env := range preferred {
		envs = append(envs, env)
	}
	sort.Strings(envs)
	result.EnvsMapped = envs

	for _, affinity := range affinities {
		node, ok := preferred[affinity.EnvGroup]
		if !ok {
			continue
		}
		rule := scheduler.AffinityRule{
			ID:       "envaffinity-" + affinity.ServerID,
			Name:     "env-affinity:" + affinity.EnvGroup,
			ServerID: affinity.ServerID,
			NodeID:   node.nodeID,
			Label:    EnvKey + "=" + affinity.EnvGroup,
			Weight:   5e10,
		}
		_ = m.scorer.RemoveAffinityRule(ctx, rule.ID)
		if err := m.scorer.AddAffinityRule(ctx, rule); err != nil {
			continue
		}
		result.ServersPinned++
		result.RulesRegistered++
	}

	m.logger.Info("patched placement constraints from env affinity",
		"serversPinned", result.ServersPinned,
		"envs", result.EnvsMapped,
	)
	return result, nil
}