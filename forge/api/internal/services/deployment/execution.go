package deployment

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

var stepStatusMapping = map[string]Status{
	StepProvision:  StatusProvisioning,
	StepHealthGate: StatusAwaitingHealth,
	StepVerify:     StatusAwaitingHealth,
	StepPromote:    StatusPromoting,
}

func (s *Service) ExecuteDeployment(ctx context.Context, deploymentID string) error {
	executorID := uuid.NewString()
	claimed, err := s.store.ClaimExecutionLease(ctx, deploymentID, executorID, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("claim lease: %w", err)
	}
	if !claimed {
		return fmt.Errorf("deployment %s is already being executed by another worker", deploymentID)
	}
	defer func() {
		if err := s.store.ReleaseExecutionLease(ctx, deploymentID); err != nil {
			slog.Error("release execution lease", "deploymentId", deploymentID, "error", err.Error())
		}
	}()

	startTime := time.Now()

	sd, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}

	deployment := toServiceDeployment(sd)

	if deployment.Status != StatusPending {
		return fmt.Errorf("deployment %s is not in pending state (current: %s)", deploymentID, deployment.Status)
	}

	if err := s.store.UpdateDeploymentStatusVersioned(ctx, deploymentID, sd.Version, string(StatusInProgress), ""); err != nil {
		return fmt.Errorf("start deployment: %w", err)
	}

	if err := s.createSteps(ctx, deploymentID, Strategy(sd.Strategy), sd.HealthGateEnabled); err != nil {
		return fmt.Errorf("create steps: %w", err)
	}

	sd, err = s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("re-fetch deployment: %w", err)
	}
	deployment = toServiceDeployment(sd)

	timeout := time.Duration(deployment.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(DefaultTimeoutSeconds) * time.Second
	}
	timeoutAt := time.Now().UTC().Add(timeout)
	if err := s.updateProgress(ctx, deploymentID, 0, 0, &timeoutAt); err != nil {
		slog.Error("update initial progress", "deploymentId", deploymentID, "error", err.Error())
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if s.publisher != nil {
		if err := s.publisher.Publish(execCtx, newDeploymentEvent("deployment_execution_started", deployment)); err != nil {
			slog.Error("publish execution started", "deploymentId", deploymentID, "error", err.Error())
		}
	}

	steps, err := s.store.ListDeploymentSteps(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("list steps: %w", err)
	}

	for _, step := range steps {
		if step.Status != string(StepStatusPending) {
			continue
		}

		if err := s.markStepStarted(ctx, step.ID); err != nil {
			slog.Error("mark step started", "stepId", step.ID, "error", err.Error())
		}
		if len(steps) > 0 {
			pct := ((step.StepNumber) * 100) / len(steps)
			if err := s.updateProgress(ctx, deploymentID, pct, step.StepNumber+1, &timeoutAt); err != nil {
				slog.Error("update progress", "deploymentId", deploymentID, "error", err.Error())
			}
		}

		stepErr := s.executeStep(execCtx, deployment, &step)

		if stepErr != nil {
			if err := s.markStepFailed(ctx, step.ID, stepErr.Error()); err != nil {
				slog.Error("mark step failed", "stepId", step.ID, "error", err.Error())
			}
			handleCtx, handleCancel := context.WithTimeout(ctx, 30*time.Second)
			s.handleStepFailure(handleCtx, deployment, stepErr)
			handleCancel()
			return stepErr
		}

		if err := s.markStepCompleted(ctx, step.ID); err != nil {
			slog.Error("mark step completed", "stepId", step.ID, "error", err.Error())
		}
	}

	duration := time.Since(startTime)
	slog.Info("deployment executed successfully",
		"deploymentId", deploymentID,
		"serverId", deployment.ServerID,
		"strategy", deployment.Strategy,
		"duration", duration.String(),
	)

	fresh, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("re-fetch deployment: %w", err)
	}
	if err := s.store.UpdateDeploymentCompletion(ctx, deploymentID, fresh.Version); err != nil {
		slog.Error("update deployment completion", "deploymentId", deploymentID, "error", err.Error())
		return fmt.Errorf("update deployment completion: %w", err)
	}

	now := time.Now().UTC()
	deployment.Status = StatusCompleted
	deployment.UpdatedAt = now
	deployment.CompletedAt = &now
	deployment.ProgressPct = 100

	if s.publisher != nil {
		if err := s.publisher.Publish(execCtx, newDeploymentEvent("deployment_completed", deployment)); err != nil {
			slog.Error("publish deployment completed", "deploymentId", deploymentID, "error", err.Error())
		}
	}

	return nil
}

func (s *Service) executeStep(ctx context.Context, deployment *Deployment, step *store.DeploymentStep) error {
	stepID := step.ID

	if intermediateStatus, ok := stepStatusMapping[step.StepName]; ok {
		if err := s.store.UpdateDeploymentStatus(ctx, deployment.ID, string(intermediateStatus), ""); err != nil {
			slog.Error("update intermediate status", "deploymentId", deployment.ID, "status", string(intermediateStatus), "error", err.Error())
		}
	}

	switch step.StepName {
	case StepInit:
		return s.executeInitStep(ctx, deployment)

	case StepProvision:
		return s.executeProvisionStep(ctx, deployment)

	case StepHealthGate:
		return s.WaitForHealthGate(ctx, deployment, stepID)

	case StepVerify:
		return s.WaitForHealthGate(ctx, deployment, stepID)

	case StepPromote:
		return s.executePromoteStep(ctx, deployment)

	case StepDrainOld:
		return s.executeDrainOldStep(ctx, deployment)

	case StepDrainCanary:
		return s.executeDrainCanaryStep(ctx, deployment)

	case StepScaleUp:
		return s.executeScaleUpStep(ctx, deployment)

	case StepScaleDown:
		return s.executeScaleDownStep(ctx, deployment)

	case StepComplete:
		return nil

	case StepCleanup:
		return s.executeCleanupStep(ctx, deployment)

	case StepRollback:
		return s.executeRollbackStep(ctx, deployment)

	default:
		return fmt.Errorf("unknown step: %s", step.StepName)
	}
}

func (s *Service) executeInitStep(ctx context.Context, deployment *Deployment) error {
	if err := validateImageRef(deployment.Image); err != nil {
		return fmt.Errorf("init step: %w", err)
	}

	revCfg := &RevisionConfig{
		ImageRef:    deployment.Image,
		Description: fmt.Sprintf("%s rollout: %s", deployment.Strategy, deployment.Image),
	}
	snapshotRev, err := s.CreateRevision(ctx, deployment.ID, revCfg)
	if err != nil {
		return fmt.Errorf("snapshot revision: %w", err)
	}

	now := time.Now().UTC()
	if err := s.store.UpdateDeploymentRevisionStatus(ctx, snapshotRev.ID, string(store.RevisionStatusActive), &now); err != nil {
		slog.Error("update revision status", "revisionId", snapshotRev.ID, "error", err.Error())
	}
	if err := s.store.UpdateDeploymentCurrentRevision(ctx, deployment.ID, &snapshotRev.ID); err != nil {
		slog.Error("update current revision", "deploymentId", deployment.ID, "error", err.Error())
	}
	deployment.CurrentRevisionID = &snapshotRev.ID

	if deployment.Strategy == StrategyBlueGreen || deployment.Strategy == StrategyCanary {
		greenID := fmt.Sprintf("%s-target-%d", deployment.ServerID, time.Now().Unix())
		fresh, err := s.store.GetDeployment(ctx, deployment.ID)
		if err != nil {
			return fmt.Errorf("re-fetch deployment: %w", err)
		}
		if err := s.store.UpdateDeploymentConfig(ctx, deployment.ID, fresh.Version, "", "", greenID, ""); err != nil {
			return fmt.Errorf("update deployment config: %w", err)
		}
		deployment.GreenTargetID = greenID
		deployment.UpdatedAt = now
	}

	return nil
}

func (s *Service) executeProvisionStep(ctx context.Context, deployment *Deployment) error {
	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, newDeploymentEvent("deployment_provisioning", deployment))
	}
	return nil
}

func (s *Service) executePromoteStep(ctx context.Context, deployment *Deployment) error {
	newTarget := "green"
	if deployment.ActiveTarget != "blue" {
		newTarget = "blue"
	}
	fresh, err := s.store.GetDeployment(ctx, deployment.ID)
	if err != nil {
		return fmt.Errorf("re-fetch deployment: %w", err)
	}
	if err := s.store.UpdateDeploymentConfig(ctx, deployment.ID, fresh.Version, "", "", "", newTarget); err != nil {
		return fmt.Errorf("update deployment config: %w", err)
	}
	deployment.ActiveTarget = newTarget
	deployment.UpdatedAt = time.Now().UTC()

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, newDeploymentEvent("deployment_promoted", deployment))
	}

	return nil
}

func (s *Service) executeDrainOldStep(ctx context.Context, deployment *Deployment) error {
	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, newDeploymentEvent("deployment_draining", deployment))
	}
	return nil
}

func (s *Service) executeDrainCanaryStep(ctx context.Context, deployment *Deployment) error {
	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, newDeploymentEvent("canary_draining", deployment))
	}
	return nil
}

func (s *Service) executeScaleUpStep(ctx context.Context, deployment *Deployment) error {
	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, newDeploymentEvent("rolling_scale_up", deployment))
	}
	return nil
}

func (s *Service) executeScaleDownStep(ctx context.Context, deployment *Deployment) error {
	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, newDeploymentEvent("rolling_scale_down", deployment))
	}
	return nil
}

func (s *Service) executeCleanupStep(ctx context.Context, deployment *Deployment) error {
	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, newDeploymentEvent("deployment_cleanup", deployment))
	}
	return nil
}

func (s *Service) executeRollbackStep(ctx context.Context, deployment *Deployment) error {
	_, err := s.RollbackToPrevious(ctx, deployment.ID)
	if err != nil {
		return fmt.Errorf("rollback step: %w", err)
	}
	return nil
}

func (s *Service) handleStepFailure(ctx context.Context, deployment *Deployment, stepErr error) {
	now := time.Now().UTC()
	fresh, err := s.store.GetDeployment(ctx, deployment.ID)
	if err == nil {
		if err := s.store.UpdateDeploymentFailure(ctx, deployment.ID, fresh.Version, stepErr.Error()); err != nil {
			slog.Error("update deployment failure", "deploymentId", deployment.ID, "error", err.Error())
		}
	}
	deployment.Status = StatusFailed
	deployment.Error = stepErr.Error()
	deployment.UpdatedAt = now
	deployment.CompletedAt = &now

	if deployment.AutoRollbackEnabled || deployment.RollbackOnHealthFailure {
		if s.publisher != nil {
			_ = s.publisher.Publish(ctx, newDeploymentEvent("auto_rollback_triggered", deployment))
		}

		if err := s.store.UpdateDeploymentStatus(ctx, deployment.ID, string(StatusRollbackPending), stepErr.Error()); err != nil {
			slog.Error("update status to rollback_pending", "deploymentId", deployment.ID, "error", err.Error())
		}

		rollbackDeploy, rollbackErr := s.RollbackToPrevious(ctx, deployment.ID)
		if rollbackErr != nil {
			slog.Error("auto-rollback failed",
				"deploymentId", deployment.ID,
				"serverId", deployment.ServerID,
				"error", rollbackErr.Error(),
			)
			if s.publisher != nil {
				_ = s.publisher.Publish(ctx, newDeploymentEvent("auto_rollback_failed", deployment))
			}
		} else {
			if err := s.store.UpdateDeploymentStatus(ctx, deployment.ID, string(StatusRollingBack), ""); err != nil {
				slog.Error("update status to rolling_back", "deploymentId", deployment.ID, "error", err.Error())
			}

			now := time.Now().UTC()
			rollbackFresh, err := s.store.GetDeployment(ctx, deployment.ID)
			if err == nil {
				if err := s.store.UpdateDeploymentRollback(ctx, deployment.ID, rollbackFresh.Version, ""); err != nil {
					slog.Error("update deployment rollback", "deploymentId", deployment.ID, "error", err.Error())
				}
				rollbackDeploy.Status = StatusRolledBack
				rollbackDeploy.CompletedAt = &now
				rollbackDeploy.UpdatedAt = now
			}

			if s.publisher != nil {
				_ = s.publisher.Publish(ctx, newDeploymentEvent("auto_rollback_completed", deployment))
			}
		}
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, newDeploymentEvent("deployment_failed", deployment))
	}

	if deployment.CleanupOnFailure {
		if s.publisher != nil {
			_ = s.publisher.Publish(ctx, newDeploymentEvent("deployment_cleanup", deployment))
		}
	}
}

func (s *Service) ResumeDeployments(ctx context.Context) error {
	s.resumeMu.Lock()
	defer s.resumeMu.Unlock()

	pending, err := s.store.ListInProgressDeployments(ctx)
	if err != nil {
		return fmt.Errorf("list in-progress deployments: %w", err)
	}

	for _, d := range pending {
		deployment := toServiceDeployment(d)
		if deployment.TimeoutAt != nil && time.Now().UTC().After(*deployment.TimeoutAt) {
			now := time.Now().UTC()
			_ = s.store.UpdateDeploymentFailure(ctx, d.ID, d.Version, "deployment timed out during API restart")
			deployment.Status = StatusFailed
			deployment.Error = "deployment timed out during API restart"
			deployment.UpdatedAt = now
			deployment.CompletedAt = &now

			if s.publisher != nil {
				_ = s.publisher.Publish(ctx, newDeploymentEvent("deployment_timed_out", deployment))
			}
			continue
		}

		if deployment.Status == StatusPending {
			if _, loaded := s.executingDeployments.LoadOrStore(deployment.ID, true); loaded {
				continue
			}
			go func(id string) {
				if err := s.ExecuteDeployment(context.Background(), id); err != nil {
					slog.Error("resume deployment failed", "deploymentId", id, "error", err.Error())
				}
			}(deployment.ID)
		}

		if deployment.Status == StatusInProgress || deployment.Status == StatusProvisioning ||
			deployment.Status == StatusAwaitingHealth || deployment.Status == StatusPromoting ||
			deployment.Status == StatusRollingBack || deployment.Status == StatusRollbackPending {
			if _, loaded := s.executingDeployments.LoadOrStore(deployment.ID, true); loaded {
				continue
			}
			go func(id string) {
				if err := s.resumeFromStep(context.Background(), id); err != nil {
					slog.Error("resume deployment from step failed", "deploymentId", id, "error", err.Error())
				}
			}(deployment.ID)
		}
	}

	return nil
}

func (s *Service) resumeFromStep(ctx context.Context, deploymentID string) error {
	executorID := uuid.NewString()
	claimed, err := s.store.ClaimExecutionLease(ctx, deploymentID, executorID, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("claim lease: %w", err)
	}
	if !claimed {
		return fmt.Errorf("deployment %s is already being executed by another worker", deploymentID)
	}
	defer s.store.ReleaseExecutionLease(ctx, deploymentID)

	sd, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	deployment := toServiceDeployment(sd)

	steps, err := s.store.ListDeploymentSteps(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("list steps: %w", err)
	}

	var startIndex int
	for i, step := range steps {
		if step.Status == string(StepStatusPending) || step.Status == string(StepStatusRunning) {
			startIndex = i

			if step.Status == string(StepStatusRunning) {
				_ = s.store.UpdateDeploymentStepStatus(ctx, step.ID, string(StepStatusPending), "")

				if step.StepName == StepInit && deployment.CurrentRevisionID != nil {
					_ = s.markStepCompleted(ctx, step.ID)
					startIndex = i + 1
					_ = s.updateProgress(ctx, deploymentID, (startIndex*100)/len(steps), startIndex, deployment.TimeoutAt)
				}
			}
			break
		}
	}

	steps, err = s.store.ListDeploymentSteps(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("list steps after resume: %w", err)
	}

	timeout := time.Duration(deployment.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(DefaultTimeoutSeconds) * time.Second
	}
	timeoutAt := time.Now().UTC().Add(timeout)
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for i := startIndex; i < len(steps); i++ {
		step := steps[i]
		if step.Status != string(StepStatusPending) {
			continue
		}

		if err := s.markStepStarted(ctx, step.ID); err != nil {
			slog.Error("mark step started", "stepId", step.ID, "error", err.Error())
		}
		if len(steps) > 0 {
			pct := (step.StepNumber * 100) / len(steps)
			if err := s.updateProgress(ctx, deploymentID, pct, step.StepNumber+1, &timeoutAt); err != nil {
				slog.Error("update progress", "deploymentId", deploymentID, "error", err.Error())
			}
		}

		stepErr := s.executeStep(execCtx, deployment, &step)

		if stepErr != nil {
			if err := s.markStepFailed(ctx, step.ID, stepErr.Error()); err != nil {
				slog.Error("mark step failed", "stepId", step.ID, "error", err.Error())
			}
			handleCtx, handleCancel := context.WithTimeout(ctx, 30*time.Second)
			s.handleStepFailure(handleCtx, deployment, stepErr)
			handleCancel()
			return stepErr
		}

		if err := s.markStepCompleted(ctx, step.ID); err != nil {
			slog.Error("mark step completed", "stepId", step.ID, "error", err.Error())
		}
	}

	fresh, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("re-fetch deployment: %w", err)
	}
	if err := s.store.UpdateDeploymentCompletion(ctx, deploymentID, fresh.Version); err != nil {
		slog.Error("update deployment completion", "deploymentId", deploymentID, "error", err.Error())
		return fmt.Errorf("update deployment completion: %w", err)
	}

	now := time.Now().UTC()
	deployment.Status = StatusCompleted
	deployment.UpdatedAt = now
	deployment.CompletedAt = &now
	deployment.ProgressPct = 100

	return nil
}

func (s *Service) CleanupFailedDeployments(ctx context.Context, olderThan time.Duration) (int, error) {
	sds, err := s.store.ListAllDeployments(ctx)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().UTC().Add(-olderThan)
	count := 0

	for _, sd := range sds {
		d := toServiceDeployment(sd)
		if (d.Status == StatusFailed || d.Status == StatusCancelled || d.Status == StatusRolledBack) && d.CreatedAt.Before(cutoff) {
			if !d.CleanupOnFailure {
				continue
			}
			_ = s.store.DeleteDeployment(ctx, d.ID)
			count++
		}
	}

	return count, nil
}

func newDeploymentEvent(eventType string, d *Deployment) events.Envelope {
	return events.NewEnvelope(
		events.EventType(eventType),
		"deployment",
		"server",
		d.ServerID,
		map[string]any{
			"deploymentId": d.ID,
			"serverId":     d.ServerID,
			"image":        d.Image,
			"strategy":     string(d.Strategy),
			"status":       string(d.Status),
		},
	)
}
