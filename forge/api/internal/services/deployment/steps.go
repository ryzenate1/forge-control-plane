package deployment

import (
	"context"
	"fmt"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

const (
	StepInit         = "init"
	StepDrainOld     = "drain_old"
	StepProvision    = "provision"
	StepHealthGate   = "health_gate"
	StepPromote      = "promote"
	StepDrainCanary  = "drain_canary"
	StepComplete     = "complete"
	StepCleanup      = "cleanup"
	StepRollback     = "rollback"
	StepScaleUp      = "scale_up"
	StepScaleDown    = "scale_down"
	StepVerify       = "verify"
)

func (s *Service) createSteps(ctx context.Context, deploymentID string, strategy Strategy, healthGateEnabled bool) error {
	stepDefs := s.stepsForStrategy(strategy, healthGateEnabled)
	for i, step := range stepDefs {
		now := time.Now().UTC()
		storeStep := &store.DeploymentStep{
			ID:           uuid.NewString(),
			DeploymentID: deploymentID,
			StepNumber:   i,
			StepName:     step,
			Status:       string(StepStatusPending),
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := s.store.CreateDeploymentStep(ctx, storeStep); err != nil {
			return fmt.Errorf("create step %d: %w", i, err)
		}
	}
	return nil
}

func (s *Service) stepsForStrategy(strategy Strategy, healthGateEnabled bool) []string {
	switch strategy {
	case StrategyRecreate:
		steps := []string{StepInit, StepProvision}
		if healthGateEnabled {
			steps = append(steps, StepHealthGate)
		}
		steps = append(steps, StepComplete)
		return steps
	case StrategyRolling:
		steps := []string{StepInit, StepScaleUp}
		if healthGateEnabled {
			steps = append(steps, StepHealthGate)
		}
		steps = append(steps, StepScaleDown, StepComplete)
		return steps
	case StrategyBlueGreen:
		steps := []string{StepInit, StepProvision}
		if healthGateEnabled {
			steps = append(steps, StepHealthGate)
		}
		steps = append(steps, StepPromote)
		if healthGateEnabled {
			steps = append(steps, StepVerify)
		}
		steps = append(steps, StepDrainOld, StepComplete)
		return steps
	case StrategyCanary:
		steps := []string{StepInit, StepProvision}
		if healthGateEnabled {
			steps = append(steps, StepHealthGate)
		}
		steps = append(steps, StepDrainCanary, StepPromote, StepComplete)
		return steps
	default:
		return []string{StepInit, StepProvision, StepComplete}
	}
}

func (s *Service) markStepStarted(ctx context.Context, stepID string) error {
	return s.store.UpdateDeploymentStepStatus(ctx, stepID, string(StepStatusRunning), "")
}

func (s *Service) markStepCompleted(ctx context.Context, stepID string) error {
	return s.store.UpdateDeploymentStepStatus(ctx, stepID, string(StepStatusCompleted), "")
}

func (s *Service) markStepFailed(ctx context.Context, stepID string, errMsg string) error {
	return s.store.UpdateDeploymentStepStatus(ctx, stepID, string(StepStatusFailed), errMsg)
}

func (s *Service) markStepSkipped(ctx context.Context, stepID string) error {
	return s.store.UpdateDeploymentStepStatus(ctx, stepID, string(StepStatusSkipped), "")
}

func (s *Service) updateProgress(ctx context.Context, deploymentID string, pct int, nextStep int, timeoutAt *time.Time) error {
	return s.store.UpdateDeploymentProgress(ctx, deploymentID, pct, nextStep, timeoutAt)
}

func (s *Service) ListSteps(ctx context.Context, deploymentID string) ([]*DeploymentStep, error) {
	storeSteps, err := s.store.ListDeploymentSteps(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	result := make([]*DeploymentStep, len(storeSteps))
	for i, st := range storeSteps {
		result[i] = toServiceStep(st)
	}
	return result, nil
}

func (s *Service) GetStep(ctx context.Context, stepID string) (*DeploymentStep, error) {
	storeStep, err := s.store.GetDeploymentStep(ctx, stepID)
	if err != nil {
		return nil, ErrStepNotFound
	}
	return toServiceStep(storeStep), nil
}
