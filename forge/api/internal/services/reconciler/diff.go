package reconciler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"gamepanel/forge/internal/store"
)

func snapshotHash(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8])
}

func computeDiffs(desired DesiredStateSnapshot, observed ObservedStateSnapshot) []ReconcileDiff {
	var diffs []ReconcileDiff

	if desired.ResourceKind != observed.ResourceKind {
		return diffs
	}

	switch desired.ResourceKind {
	case ResourceKindServer:
		diffs = append(diffs, computeServerDiffs(desired, observed)...)
	case ResourceKindNode:
		diffs = append(diffs, computeNodeDiffs(desired, observed)...)
	case ResourceKindComposeStack:
		diffs = append(diffs, computeComposeDiffs(desired, observed)...)
	}

	return diffs
}

func computeServerDiffs(desired DesiredStateSnapshot, observed ObservedStateSnapshot) []ReconcileDiff {
	var diffs []ReconcileDiff

	if desired.ServerState == nil && observed.ServerState == nil {
		return diffs
	}

	if desired.ServerState != nil && observed.ServerState == nil {
		diffs = append(diffs, ReconcileDiff{
			ResourceID:   desired.ResourceID,
			ResourceKind: ResourceKindServer,
			DiffType:     DiffCreate,
			DesiredHash:  desired.ConfigHash,
			Description:  fmt.Sprintf("server %s: desired %s, no observed state (create)", desired.ResourceID, *desired.ServerState),
		})
		return diffs
	}

	if desired.ServerState == nil && observed.ServerState != nil {
		diffs = append(diffs, ReconcileDiff{
			ResourceID:   observed.ResourceID,
			ResourceKind: ResourceKindServer,
			DiffType:     DiffDelete,
			ObservedHash: observed.ConfigHash,
			Description:  fmt.Sprintf("server %s: no desired state, observed %s (delete)", observed.ResourceID, *observed.ServerState),
		})
		return diffs
	}

	if string(*desired.ServerState) != string(*observed.ServerState) {
		diffType := DiffUpdate
		desc := fmt.Sprintf("server %s: desired %s, observed %s",
			desired.ResourceID, *desired.ServerState, *observed.ServerState)

		if desired.ConfigHash != observed.ConfigHash {
			desc += " (config changed)"
		}

		diffs = append(diffs, ReconcileDiff{
			ResourceID:   desired.ResourceID,
			ResourceKind: ResourceKindServer,
			DiffType:     diffType,
			DesiredHash:  desired.ConfigHash,
			ObservedHash: observed.ConfigHash,
			Description:  desc,
			Details: map[string]any{
				"desiredState":  string(*desired.ServerState),
				"observedState": string(*observed.ServerState),
				"configChanged": desired.ConfigHash != observed.ConfigHash,
			},
		})
	} else if desired.ConfigHash != observed.ConfigHash {
		diffs = append(diffs, ReconcileDiff{
			ResourceID:   desired.ResourceID,
			ResourceKind: ResourceKindServer,
			DiffType:     DiffUpdate,
			DesiredHash:  desired.ConfigHash,
			ObservedHash: observed.ConfigHash,
			Description:  fmt.Sprintf("server %s: state matches but configuration differs", desired.ResourceID),
			Details: map[string]any{
				"desiredState":  string(*desired.ServerState),
				"observedState": string(*observed.ServerState),
				"configChanged": true,
			},
		})
	} else {
		diffs = append(diffs, ReconcileDiff{
			ResourceID:   desired.ResourceID,
			ResourceKind: ResourceKindServer,
			DiffType:     DiffNoOp,
			DesiredHash:  desired.ConfigHash,
			ObservedHash: observed.ConfigHash,
			Description:  fmt.Sprintf("server %s: in sync", desired.ResourceID),
		})
	}

	return diffs
}

func computeNodeDiffs(desired DesiredStateSnapshot, observed ObservedStateSnapshot) []ReconcileDiff {
	var diffs []ReconcileDiff

	if desired.NodeState != nil && observed.NodeState == nil {
		diffs = append(diffs, ReconcileDiff{
			ResourceID:   desired.ResourceID,
			ResourceKind: ResourceKindNode,
			DiffType:     DiffCreate,
			DesiredHash:  desired.ConfigHash,
			Description:  fmt.Sprintf("node %s: desired %s, no observed state (register)", desired.ResourceID, *desired.NodeState),
		})
		return diffs
	}

	if desired.NodeState == nil && observed.NodeState != nil {
		diffs = append(diffs, ReconcileDiff{
			ResourceID:   observed.ResourceID,
			ResourceKind: ResourceKindNode,
			DiffType:     DiffDelete,
			ObservedHash: observed.ConfigHash,
			Description:  fmt.Sprintf("node %s: no desired state, observed %s (deregister)", observed.ResourceID, *observed.NodeState),
		})
		return diffs
	}

	if desired.NodeState != nil && observed.NodeState != nil {
		desiredStr := string(*desired.NodeState)
		observedStr := *observed.NodeState

		if desiredStr != observedStr {
			diffs = append(diffs, ReconcileDiff{
				ResourceID:   desired.ResourceID,
				ResourceKind: ResourceKindNode,
				DiffType:     DiffUpdate,
				DesiredHash:  desired.ConfigHash,
				ObservedHash: observed.ConfigHash,
				Description:  fmt.Sprintf("node %s: desired %s, observed %s", desired.ResourceID, desiredStr, observedStr),
				Details: map[string]any{
					"desiredState":  desiredStr,
					"observedState": observedStr,
				},
			})
		} else if desired.ConfigHash != observed.ConfigHash {
			diffs = append(diffs, ReconcileDiff{
				ResourceID:   desired.ResourceID,
				ResourceKind: ResourceKindNode,
				DiffType:     DiffUpdate,
				DesiredHash:  desired.ConfigHash,
				ObservedHash: observed.ConfigHash,
				Description:  fmt.Sprintf("node %s: state matches but configuration differs", desired.ResourceID),
				Details: map[string]any{
					"configChanged": true,
				},
			})
		} else {
			diffs = append(diffs, ReconcileDiff{
				ResourceID:   desired.ResourceID,
				ResourceKind: ResourceKindNode,
				DiffType:     DiffNoOp,
				DesiredHash:  desired.ConfigHash,
				ObservedHash: observed.ConfigHash,
				Description:  fmt.Sprintf("node %s: in sync", desired.ResourceID),
			})
		}
	}

	return diffs
}

func computeComposeDiffs(desired DesiredStateSnapshot, observed ObservedStateSnapshot) []ReconcileDiff {
	var diffs []ReconcileDiff
	resourceID := desired.ResourceID

	if desired.ComposeState == nil && observed.ComposeState == nil {
		diffs = append(diffs, ReconcileDiff{
			ResourceID:   resourceID,
			ResourceKind: ResourceKindComposeStack,
			DiffType:     DiffNoOp,
			Description:  fmt.Sprintf("compose stack %s: no state available", resourceID),
		})
		return diffs
	}

	if desired.ComposeState != nil && observed.ComposeState == nil {
		diffs = append(diffs, ReconcileDiff{
			ResourceID:   resourceID,
			ResourceKind: ResourceKindComposeStack,
			DiffType:     DiffCreate,
			DesiredHash:  desired.ConfigHash,
			Description:  fmt.Sprintf("compose stack %s: desired exists, no observed state (create)", resourceID),
		})
		return diffs
	}

	if desired.ComposeState == nil && observed.ComposeState != nil {
		diffs = append(diffs, ReconcileDiff{
			ResourceID:   resourceID,
			ResourceKind: ResourceKindComposeStack,
			DiffType:     DiffDelete,
			ObservedHash: observed.ConfigHash,
			Description:  fmt.Sprintf("compose stack %s: no desired state, observed exists (delete)", resourceID),
		})
		return diffs
	}

	desiredStatus := desired.ComposeState.Status
	observedStatus := observed.ComposeState.Status
	hashChanged := desired.ConfigHash != observed.ConfigHash

	if desiredStatus != observedStatus || hashChanged {
		desc := fmt.Sprintf("compose stack %s: desired status %s, observed status %s",
			resourceID, desiredStatus, observedStatus)
		if hashChanged {
			desc += " (config changed)"
		}
		diffs = append(diffs, ReconcileDiff{
			ResourceID:   resourceID,
			ResourceKind: ResourceKindComposeStack,
			DiffType:     DiffUpdate,
			DesiredHash:  desired.ConfigHash,
			ObservedHash: observed.ConfigHash,
			Description:  desc,
			Details: map[string]any{
				"desiredStatus":  desiredStatus,
				"observedStatus": observedStatus,
				"configChanged":  hashChanged,
			},
		})
	} else {
		diffs = append(diffs, ReconcileDiff{
			ResourceID:   resourceID,
			ResourceKind: ResourceKindComposeStack,
			DiffType:     DiffNoOp,
			DesiredHash:  desired.ConfigHash,
			ObservedHash: observed.ConfigHash,
			Description:  fmt.Sprintf("compose stack %s: in sync", resourceID),
		})
	}

	return diffs
}

func collectComposeStackConfigHash(stack ComposeDesiredState) string {
	return snapshotHash(map[string]any{
		"composeYaml": stack.ComposeYAML,
		"composeHash": stack.ComposeHash,
		"status":      stack.Status,
	})
}

func generatePlan(resourceID string, resourceKind ResourceKind, diffs []ReconcileDiff, drifts []DriftRecord) *ReconcilePlan {
	plan := &ReconcilePlan{
		ResourceID:   resourceID,
		ResourceKind: resourceKind,
		Diffs:        diffs,
		Drifts:       drifts,
	}

	for _, diff := range diffs {
		if diff.DiffType == DiffDelete {
			plan.Destructive = true
			break
		}
	}

	return plan
}

type ResourceDiffSummary struct {
	TotalDiffs    int `json:"totalDiffs"`
	Creates       int `json:"creates"`
	Updates       int `json:"updates"`
	Deletes       int `json:"deletes"`
	NoOps         int `json:"noOps"`
	TotalDrifts   int `json:"totalDrifts"`
	ConfigDrifts  int `json:"configDrifts"`
	Missing       int `json:"missing"`
	Orphans       int `json:"orphans"`
}

func summarizeResourceState(servers []store.Server, nodes []store.Node) map[string]ResourceDiffSummary {
	summary := map[string]ResourceDiffSummary{}

	for range servers {
		// For each server compute whether desired == observed
		// This is a summary, so we aggregate
	}

	for range nodes {
		// Same for nodes
	}

	return summary
}

func collectServerConfigHash(server store.Server) string {
	return snapshotHash(map[string]any{
		"memoryMb":       server.MemoryMB,
		"cpuShares":      server.CPUShares,
		"diskMb":         server.DiskMB,
		"dockerImage":    server.DockerImage,
		"startupCommand": server.StartupCommand,
		"suspended":      server.Suspended,
	})
}

func diffSummaries(diffs []ReconcileDiff) ResourceDiffSummary {
	var s ResourceDiffSummary
	s.TotalDiffs = len(diffs)
	for _, d := range diffs {
		switch d.DiffType {
		case DiffCreate:
			s.Creates++
		case DiffUpdate:
			s.Updates++
		case DiffDelete:
			s.Deletes++
		case DiffNoOp:
			s.NoOps++
		}
	}
	return s
}

func driftSummaries(drifts []DriftRecord) (config, missing, orphan int) {
	for _, d := range drifts {
		switch d.DriftKind {
		case DriftConfigMismatch:
			config++
		case DriftMissingResource:
			missing++
		case DriftOrphanedResource:
			orphan++
		}
	}
	return
}

func sortDiffsByType(diffs []ReconcileDiff) {
	sort.Slice(diffs, func(i, j int) bool {
		order := map[ReconcileDiffType]int{
			DiffDelete: 0,
			DiffUpdate: 1,
			DiffCreate: 2,
			DiffNoOp:   3,
		}
		oi, oj := order[diffs[i].DiffType], order[diffs[j].DiffType]
		if oi != oj {
			return oi < oj
		}
		return diffs[i].ResourceID < diffs[j].ResourceID
	})
}
