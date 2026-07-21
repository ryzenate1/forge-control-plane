package recovery

import (
	"context"
	"errors"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type RecoveryStatusService struct {
	store     *store.Store
	publisher events.Publisher
}

func NewRecoveryStatusService(store *store.Store, publishers ...events.Publisher) *RecoveryStatusService {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &RecoveryStatusService{store: store, publisher: publisher}
}

type StatusMetric struct {
	TotalPlans    int `json:"totalPlans"`
	ActivePlans   int `json:"activePlans"`
	FailedPlans   int `json:"failedPlans"`
	CompletedPlans int `json:"completedPlans"`
	TotalItems     int `json:"totalItems"`
}

func (s *RecoveryStatusService) Summary(ctx context.Context) (StatusMetric, error) {
	if s == nil || s.store == nil {
		return StatusMetric{}, errors.New("recovery status service unavailable")
	}
	plans, err := s.store.ListRecoveryPlans(ctx)
	if err != nil {
		return StatusMetric{}, err
	}
	var metric StatusMetric
	metric.TotalPlans = len(plans)
	for _, plan := range plans {
		metric.TotalItems += len(plan.Items)
		switch plan.Status {
		case store.RecoveryPlanStatusPending, store.RecoveryPlanStatusPlanning,
			store.RecoveryPlanStatusPlanned, store.RecoveryPlanStatusExecuting:
			metric.ActivePlans++
		case store.RecoveryPlanStatusFailed:
			metric.FailedPlans++
		case store.RecoveryPlanStatusCompleted, store.RecoveryPlanStatusRestored:
			metric.CompletedPlans++
		}
	}
	return metric, nil
}

type RecoveryStatus struct {
	LastRecoveryTime *time.Time `json:"lastRecoveryTime,omitempty"`
	ActiveRecoveries int        `json:"activeRecoveries"`
	LastError        string     `json:"lastError,omitempty"`
}

func (s *RecoveryStatusService) NodeRecoveryStatus(ctx context.Context, nodeID string) (RecoveryStatus, error) {
	if s == nil || s.store == nil {
		return RecoveryStatus{}, errors.New("recovery status service unavailable")
	}
	plans, err := s.store.ListRecoveryPlans(ctx)
	if err != nil {
		return RecoveryStatus{}, err
	}
	var status RecoveryStatus
	for _, plan := range plans {
		if plan.NodeID != nodeID {
			continue
		}
		if plan.Status == store.RecoveryPlanStatusPending ||
			plan.Status == store.RecoveryPlanStatusPlanning ||
			plan.Status == store.RecoveryPlanStatusPlanned ||
			plan.Status == store.RecoveryPlanStatusExecuting {
			status.ActiveRecoveries++
		}
		if plan.Status == store.RecoveryPlanStatusFailed {
			status.LastError = plan.Reason
		}
		if plan.Status == store.RecoveryPlanStatusCompleted ||
			plan.Status == store.RecoveryPlanStatusRestored {
			t := plan.UpdatedAt
			if status.LastRecoveryTime == nil || t.After(*status.LastRecoveryTime) {
				status.LastRecoveryTime = &t
			}
		}
	}
	if status.LastError == "" && status.ActiveRecoveries == 0 && status.LastRecoveryTime == nil {
		return RecoveryStatus{}, nil
	}
	return status, nil
}

type NodeRecoverySummary struct {
	NodeID           string           `json:"nodeId"`
	HeartbeatState   string           `json:"heartbeatState"`
	ActualState      string           `json:"actualState"`
	RecoveryStatus   *RecoveryStatus  `json:"recoveryStatus,omitempty"`
	OrphanedWorkloads int             `json:"orphanedWorkloads,omitempty"`
}
