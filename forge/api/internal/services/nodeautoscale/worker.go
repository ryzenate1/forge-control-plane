package nodeautoscale

import (
	"context"
	"encoding/json"
	"time"

	"gamepanel/forge/internal/store"
)

// Start launches the periodic (60s) autoscaler scan. Each enabled policy is
// evaluated; total cooldown is honored per policy; AutoJoin policies
// provision immediately, everything else is recorded as a suggestion.
// Errors are recorded in the audit ledger, never fatal — the scan continues.
func (s *Service) Start(ctx context.Context) {
	if s == nil || s.store == nil {
		return
	}
	ticker := time.NewTicker(60 * time.Second)
	go func() {
		defer ticker.Stop()
		// First pass shortly after boot.
		first := time.NewTimer(5 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-first.C:
				s.scanOnce(ctx)
				first.Stop()
			case <-ticker.C:
				s.scanOnce(ctx)
			}
		}
	}()
	s.logger.Info("node autoscaler scan started", "interval", "60s")
}

func (s *Service) scanOnce(ctx context.Context) {
	policies, err := s.store.ListNodeAutoscalePolicies(ctx)
	if err != nil {
		s.logger.Warn("autoscaler scan could not list policies", "error", err)
		return
	}
	for _, policy := range policies {
		if !policy.Enabled || !policy.AutoJoin {
			continue
		}
		evaluation, err := s.evaluate(ctx, policy)
		if err != nil {
			continue
		}
		if !evaluation.ScaleOut {
			continue
		}
		if s.cloud == nil {
			_ = s.auditSuggestion(ctx, policy, evaluation)
			continue
		}
		instance, err := s.provision(ctx, policy)
		if err != nil {
			raw, _ := json.Marshal(map[string]any{"error": err.Error(), "autoJoin": true})
			_ = s.store.RecordNodeAutoscaleEvent(ctx, &store.NodeAutoscaleEvent{
				PolicyID: policy.ID,
				Action:   "scale-out",
				State:    "failed",
				Deficit:  evaluation.Deficit,
				Detail:   raw,
			})
			continue
		}
		raw, _ := json.Marshal(map[string]any{
			"instanceId": instance.ID,
			"state":      "join_pending",
			"autoJoin":   true,
		})
		_ = s.store.RecordNodeAutoscaleEvent(ctx, &store.NodeAutoscaleEvent{
			PolicyID:  policy.ID,
			Action:    "scale-out",
			Direction: "out",
			State:     "applied",
			Deficit:   evaluation.Deficit,
			Detail:    raw,
		})
		s.logger.Info("autoscaler provisioned node",
			"policy", policy.Name,
			"instance", instance.ID)
	}
}

func (s *Service) auditSuggestion(ctx context.Context, policy store.NodeAutoscalePolicy, evaluation Evaluation) error {
	raw, _ := json.Marshal(map[string]any{
		"loadCpu": evaluation.Load.LoadCPU,
		"loadMem": evaluation.Load.LoadMemory,
		"autoJoin": false,
	})
	return s.store.RecordNodeAutoscaleEvent(ctx, &store.NodeAutoscaleEvent{
		PolicyID:  policy.ID,
		Action:    "scale-out",
		Direction: "out",
		State:     "suggested",
		Deficit:   evaluation.Deficit,
		Detail:    raw,
	})
}