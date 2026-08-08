package reconciler

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) detectDrift(ctx context.Context, diffs []ReconcileDiff) []DriftRecord {
	var drifts []DriftRecord

	for _, diff := range diffs {
		switch diff.DiffType {
		case DiffUpdate:
			if diff.Details != nil {
				configChanged, _ := diff.Details["configChanged"].(bool)
				if configChanged {
					drifts = append(drifts, DriftRecord{
						ResourceID:   diff.ResourceID,
						ResourceKind: diff.ResourceKind,
						DriftKind:    DriftConfigMismatch,
						Desired:      diff.DesiredHash,
						Observed:     diff.ObservedHash,
						Severity:     "warning",
						DetectedAt:   time.Now().UTC(),
						Details: map[string]any{
							"desiredState":  diff.Details["desiredState"],
							"observedState": diff.Details["observedState"],
						},
					})
				}
			}
			stateChanged := false
			if diff.Details != nil {
				ds, _ := diff.Details["desiredState"].(string)
				os, _ := diff.Details["observedState"].(string)
				if ds == "" {
					ds, _ = diff.Details["desiredStatus"].(string)
				}
				if os == "" {
					os, _ = diff.Details["observedStatus"].(string)
				}
				stateChanged = ds != "" && os != "" && ds != os
			}
			if stateChanged {
				drifts = append(drifts, DriftRecord{
					ResourceID:   diff.ResourceID,
					ResourceKind: diff.ResourceKind,
					DriftKind:    DriftStateMismatch,
					Desired:      fmt.Sprintf("%v", diff.Details["desiredState"]),
					Observed:     fmt.Sprintf("%v", diff.Details["observedState"]),
					Severity:     "high",
					DetectedAt:   time.Now().UTC(),
				})
			}
		case DiffCreate:
			drifts = append(drifts, DriftRecord{
				ResourceID:   diff.ResourceID,
				ResourceKind: diff.ResourceKind,
				DriftKind:    DriftMissingResource,
				Desired:      "exists",
				Observed:     "not_found",
				Severity:     "critical",
				DetectedAt:   time.Now().UTC(),
			})
		case DiffDelete:
			drifts = append(drifts, DriftRecord{
				ResourceID:   diff.ResourceID,
				ResourceKind: diff.ResourceKind,
				DriftKind:    DriftOrphanedResource,
				Desired:      "absent",
				Observed:     "exists",
				Severity:     "high",
				DetectedAt:   time.Now().UTC(),
			})
		}
	}

	return drifts
}

func (s *Service) ensureNoDuplicateReconcile(ctx context.Context, resourceID string, resourceKind ResourceKind) bool {
	plans, err := s.store.ListReconcilePlansByResource(ctx, resourceID, string(resourceKind))
	if err != nil {
		return false
	}
	for _, plan := range plans {
		if plan.State == string(PlanPending) || plan.State == string(PlanExecuting) {
			return false
		}
	}
	return true
}

func classifyDrift(drift DriftRecord) string {
	switch drift.DriftKind {
	case DriftConfigMismatch:
		return "configuration drift"
	case DriftMissingResource:
		return "missing resource"
	case DriftOrphanedResource:
		return "orphaned resource"
	case DriftStateMismatch:
		return "state mismatch"
	default:
		return "unknown"
	}
}

func isDestructiveAction(diff ReconcileDiff) bool {
	return diff.DiffType == DiffDelete
}

func requiresConfirmation(diffs []ReconcileDiff) bool {
	for _, d := range diffs {
		if isDestructiveAction(d) {
			return true
		}
	}
	return false
}

func hasClustersDiverged(desiredGen, observedGen int64) bool {
	return observedGen < desiredGen
}
