// Package envaffinity wires the placement engine's dormant NodeLabels into
// live, env-aware placement. nodes.labels (JSONB) and nodes.env_groups
// (migration 190) are read from the store and injected into
// placement.ConstraintContext so label affinity constraints actually match;
// servers pinned via servers.env_affinity get hard env pinning during
// placement, and an "affinity viewer" endpoint explains why a node won or
// lost for a given workload.
package envaffinity

import (
	"context"
	"log/slog"
	"strings"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/placement"
	"gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/store"
)

// EnvKey is the synthetic label key under which env_groups membership is
// exposed to constraint checks alongside the raw node labels.
const EnvKey = "env_group"

// EnvAffinity reads node labels / env groups and server env declarations.
type EnvAffinity struct {
	store  *store.Store
	scorer *scheduler.PredictiveScorer
	logger *slog.Logger
}

func New(st *store.Store, logger *slog.Logger) *EnvAffinity {
	if logger == nil {
		logger = slog.Default()
	}
	return &EnvAffinity{store: st, logger: logger}
}

// WithPredictiveScorer enables PatchPlacementConstraints rule sync.
func (m *EnvAffinity) WithPredictiveScorer(scorer *scheduler.PredictiveScorer) *EnvAffinity {
	m.scorer = scorer
	return m
}

// EnrichedPlacement is a PlacementRequest decorated with the env-affinity
// facts a placement caller can hand to the engine.
type EnrichedPlacement struct {
	Request    domain.PlacementRequest      `json:"request"`
	EnvGroup   string                       `json:"envGroup,omitempty"`
	Constraints []placement.Constraint      `json:"constraints,omitempty"`
	Ctx        placement.ConstraintContext  `json:"-"`
}

// envMetadataKey carries an optional per-request env hint.
type envMetadataKey struct{}

// WithEnv attaches an env group hint to a placement request context.
func WithEnv(ctx context.Context, env string) context.Context {
	return context.WithValue(ctx, envMetadataKey{}, env)
}

// NodeLabelIndex builds the map[nodeID]map[key]value index that
// placement.ConstraintChecker.checkLabel expects, merging node labels from
// the nodes table with the synthetic env_group label from env_groups.
func (m *EnvAffinity) NodeLabelIndex(ctx context.Context) (map[string]map[string]string, error) {
	nodes, err := m.store.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	index := make(map[string]map[string]string, len(nodes))
	for _, node := range nodes {
		labels := make(map[string]string, len(node.Labels)+1)
		for _, pair := range node.Labels {
			labels[pair.Key] = pair.Value
		}
		index[node.ID] = labels
	}
	groups, err := m.store.ListNodeEnvGroups(ctx)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		labels, ok := index[g.NodeID]
		if !ok {
			labels = map[string]string{}
			index[g.NodeID] = labels
		}
		if len(g.EnvGroups) > 0 {
			labels[EnvKey] = strings.Join(g.EnvGroups, ",")
		}
	}
	return index, nil
}

// ConstraintContext builds the full context (server map + node labels) that
// the scheduler otherwise leaves half-populated.
func (m *EnvAffinity) ConstraintContext(ctx context.Context) (placement.ConstraintContext, error) {
	labels, err := m.NodeLabelIndex(ctx)
	if err != nil {
		return placement.ConstraintContext{}, err
	}
	serverNodeMap := map[string]string{}
	servers, err := m.store.ListServers(ctx)
	if err != nil {
		return placement.ConstraintContext{}, err
	}
	for _, server := range servers {
		serverNodeMap[server.ID] = server.Node
	}
	return placement.ConstraintContext{ServerNodeMap: serverNodeMap, NodeLabels: labels}, nil
}

// EnrichPlacementRequest pins org/project/env affinity onto a placement
// request. When the target server declares env_affinity (or the context
// carries a WithEnv hint), the returned constraints add a hard label
// constraint env_group in {env} so only nodes advertising that environment
// group remain candidates.
func (m *EnvAffinity) EnrichPlacementRequest(ctx context.Context, req domain.PlacementRequest) (EnrichedPlacement, error) {
	enriched := EnrichedPlacement{Request: req}
	ctxValue, err := m.ConstraintContext(ctx)
	if err != nil {
		return enriched, err
	}
	enriched.Ctx = ctxValue

	env := ""
	if req.ServerID != "" {
		affinities, err := m.store.ListServerEnvAffinities(ctx)
		if err == nil {
			for _, affinity := range affinities {
				if affinity.ServerID == req.ServerID && affinity.EnvGroup != "" {
					env = affinity.EnvGroup
					break
				}
			}
		}
	}
	if env == "" {
		if hint, ok := ctx.Value(envMetadataKey{}).(string); ok {
			env = hint
		}
	}
	if env == "" {
		return enriched, nil
	}

	enriched.EnvGroup = env
	enriched.Constraints = []placement.Constraint{{
		Type:     placement.ConstraintLabel,
		Operator: "in",
		Key:      EnvKey,
		Values:   []string{env},
		Required: true,
	}}
	m.logger.Info("placement enriched with env affinity",
		"serverID", req.ServerID,
		"env", env,
	)
	return enriched, nil
}

// NodeSatisfiesEnv reports whether a node advertises the env group.
func NodeSatisfiesEnv(labels map[string]map[string]string, nodeID, env string) bool {
	node, ok := labels[nodeID]
	if !ok {
		return false
	}
	for _, group := range strings.Split(node[EnvKey], ",") {
		if strings.TrimSpace(group) == env {
			return true
		}
	}
	return false
}