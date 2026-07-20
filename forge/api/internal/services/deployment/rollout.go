package deployment

import (
	"context"
	"fmt"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type RolloutRequest struct {
	ServerID        string   `json:"serverId"`
	Strategy        Strategy `json:"strategy"`
	Image           string   `json:"image"`
	HealthCheckPath string   `json:"healthCheckPath,omitempty"`
	HealthCheckPort int      `json:"healthCheckPort,omitempty"`
	CanaryPercent   int      `json:"canaryPercent,omitempty"`
}

type RolloutResult struct {
	DeploymentID string `json:"deploymentId"`
	Status       string `json:"status"`
	Message      string `json:"message"`
}

func (s *Service) StartRollout(ctx context.Context, req *RolloutRequest) (*Deployment, error) {
	if req.ServerID == "" {
		return nil, ErrInvalidServer
	}
	if req.Image == "" {
		return nil, ErrInvalidImage
	}
	if req.Strategy == "" {
		req.Strategy = StrategyRecreate
	}

	switch req.Strategy {
	case StrategyRecreate:
		return s.recreateRollout(ctx, req)
	case StrategyRolling:
		return s.rollingRollout(ctx, req)
	case StrategyBlueGreen:
		return s.blueGreenRollout(ctx, req)
	case StrategyCanary:
		return s.canaryRollout(ctx, req)
	default:
		return nil, fmt.Errorf("unknown rollout strategy: %s", req.Strategy)
	}
}

func (s *Service) recreateRollout(ctx context.Context, req *RolloutRequest) (*Deployment, error) {
	existing, err := s.store.ListDeployments(ctx, req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("check existing: %w", err)
	}
	for _, d := range existing {
		if d.Status == string(StatusInProgress) {
			return nil, ErrInProgress
		}
	}

	now := time.Now().UTC()
	deployment := &Deployment{
		ID:              uuid.NewString(),
		ServerID:        req.ServerID,
		Strategy:        StrategyRecreate,
		Status:          StatusInProgress,
		Image:           req.Image,
		HealthCheckPath: req.HealthCheckPath,
		HealthCheckPort: req.HealthCheckPort,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.store.CreateDeployment(ctx, toStoreDeployment(deployment)); err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	if err := s.store.UpdateDeploymentRolloutStrategy(ctx, deployment.ID, string(deployment.Strategy)); err != nil {
		return nil, fmt.Errorf("set rollout strategy: %w", err)
	}

	revCfg := &RevisionConfig{
		ImageRef:    req.Image,
		Description: "recreate rollout: " + req.Image,
	}
	snapshotRev, err := s.CreateRevision(ctx, deployment.ID, revCfg)
	if err != nil {
		return nil, fmt.Errorf("snapshot revision: %w", err)
	}

	now = time.Now().UTC()
	_ = s.store.UpdateDeploymentRevisionStatus(ctx, snapshotRev.ID, string(store.RevisionStatusActive), &now)
	_ = s.store.UpdateDeploymentCurrentRevision(ctx, deployment.ID, &snapshotRev.ID)

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("deployment_started", "deployment", "server", req.ServerID, map[string]any{
			"deploymentId": deployment.ID, "image": req.Image, "strategy": deployment.Strategy,
		}))
	}

	return deployment, nil
}

func (s *Service) rollingRollout(ctx context.Context, req *RolloutRequest) (*Deployment, error) {
	existing, err := s.store.ListDeployments(ctx, req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("check existing: %w", err)
	}
	for _, d := range existing {
		if d.Status == string(StatusInProgress) {
			return nil, ErrInProgress
		}
	}

	now := time.Now().UTC()
	deployment := &Deployment{
		ID:              uuid.NewString(),
		ServerID:        req.ServerID,
		Strategy:        StrategyRolling,
		Status:          StatusInProgress,
		Image:           req.Image,
		HealthCheckPath: req.HealthCheckPath,
		HealthCheckPort: req.HealthCheckPort,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.store.CreateDeployment(ctx, toStoreDeployment(deployment)); err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	if err := s.store.UpdateDeploymentRolloutStrategy(ctx, deployment.ID, string(deployment.Strategy)); err != nil {
		return nil, fmt.Errorf("set rollout strategy: %w", err)
	}

	revCfg := &RevisionConfig{
		ImageRef:    req.Image,
		Description: "rolling rollout: " + req.Image,
	}
	snapshotRev, err := s.CreateRevision(ctx, deployment.ID, revCfg)
	if err != nil {
		return nil, fmt.Errorf("snapshot revision: %w", err)
	}

	now = time.Now().UTC()
	_ = s.store.UpdateDeploymentRevisionStatus(ctx, snapshotRev.ID, string(store.RevisionStatusActive), &now)
	_ = s.store.UpdateDeploymentCurrentRevision(ctx, deployment.ID, &snapshotRev.ID)

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("rolling_update_started", "deployment", "server", req.ServerID, map[string]any{
			"deploymentId": deployment.ID, "image": req.Image, "strategy": deployment.Strategy,
		}))
	}

	return deployment, nil
}

func (s *Service) blueGreenRollout(ctx context.Context, req *RolloutRequest) (*Deployment, error) {
	return s.StartBlueGreen(ctx, req.ServerID, req.Image, req.HealthCheckPath, req.HealthCheckPort)
}

func (s *Service) canaryRollout(ctx context.Context, req *RolloutRequest) (*Deployment, error) {
	existing, err := s.store.ListDeployments(ctx, req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("check existing: %w", err)
	}
	for _, d := range existing {
		if d.Status == string(StatusInProgress) {
			return nil, ErrInProgress
		}
	}

	canaryPct := req.CanaryPercent
	if canaryPct <= 0 {
		canaryPct = 10
	}

	now := time.Now().UTC()
	deployment := &Deployment{
		ID:              uuid.NewString(),
		ServerID:        req.ServerID,
		Strategy:        StrategyCanary,
		Status:          StatusInProgress,
		Image:           req.Image,
		BlueTargetID:    fmt.Sprintf("%s-stable", req.ServerID),
		GreenTargetID:   fmt.Sprintf("%s-canary-%d", req.ServerID, time.Now().Unix()),
		ActiveTarget:    "blue",
		HealthCheckPath: req.HealthCheckPath,
		HealthCheckPort: req.HealthCheckPort,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.store.CreateDeployment(ctx, toStoreDeployment(deployment)); err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	if err := s.store.UpdateDeploymentRolloutStrategy(ctx, deployment.ID, string(deployment.Strategy)); err != nil {
		return nil, fmt.Errorf("set rollout strategy: %w", err)
	}

	revCfg := &RevisionConfig{
		ImageRef:    req.Image,
		Description: fmt.Sprintf("canary rollout (%d%%): %s", canaryPct, req.Image),
		Metadata: map[string]any{
			"canaryPercent": canaryPct,
		},
	}
	snapshotRev, err := s.CreateRevision(ctx, deployment.ID, revCfg)
	if err != nil {
		return nil, fmt.Errorf("snapshot revision: %w", err)
	}

	now = time.Now().UTC()
	_ = s.store.UpdateDeploymentRevisionStatus(ctx, snapshotRev.ID, string(store.RevisionStatusActive), &now)
	_ = s.store.UpdateDeploymentCurrentRevision(ctx, deployment.ID, &snapshotRev.ID)

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("canary_update_started", "deployment", "server", req.ServerID, map[string]any{
			"deploymentId": deployment.ID, "image": req.Image, "strategy": deployment.Strategy,
			"canaryPercent": canaryPct,
		}))
	}

	return deployment, nil
}
