package healthcheckrunner

import (
	"context"

	"gamepanel/forge/internal/services/reconciler"
)

type reconcilerAdapter struct {
	svc *Service
}

func (a *reconcilerAdapter) ListUnhealthyTargets(ctx context.Context) []reconciler.HealthCheckTarget {
	states := a.svc.ListUnhealthyTargets(ctx)
	result := make([]reconciler.HealthCheckTarget, 0, len(states))
	for _, state := range states {
		result = append(result, reconciler.HealthCheckTarget{
			TargetID:     state.ID,
			ServerID:     state.ServerID,
			Status:       string(state.Status),
			FailureCount: state.ConsecutiveFailures,
		})
	}
	return result
}

func (s *Service) ReconcilerAdapter() reconciler.HealthChecker {
	return &reconcilerAdapter{svc: s}
}
