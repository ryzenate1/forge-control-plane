package deployment

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gamepanel/forge/internal/events"

	"github.com/google/uuid"
)

type RolloutRequest struct {
	ServerID                string   `json:"serverId"`
	Strategy                Strategy `json:"strategy"`
	Image                   string   `json:"image"`
	HealthCheckPath         string   `json:"healthCheckPath,omitempty"`
	HealthCheckPort         int      `json:"healthCheckPort,omitempty"`
	HealthCheckHost         string   `json:"healthCheckHost,omitempty"`
	CanaryPercent           int      `json:"canaryPercent,omitempty"`
	TimeoutSeconds          int      `json:"timeoutSeconds,omitempty"`
	HealthGateEnabled       bool     `json:"healthGateEnabled,omitempty"`
	HealthGateThreshold     int      `json:"healthGateThreshold,omitempty"`
	HealthGateIntervalMs    int      `json:"healthGateIntervalMs,omitempty"`
	AutoRollbackEnabled     bool     `json:"autoRollbackEnabled,omitempty"`
	RollbackOnHealthFailure bool     `json:"rollbackOnHealthFailure,omitempty"`
	CleanupOnFailure        bool     `json:"cleanupOnFailure,omitempty"`
	TargetReplicas          int      `json:"targetReplicas,omitempty"`
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
	if err := validateImageRef(req.Image); err != nil {
		return nil, err
	}
	req.Image = digestImageRef(req.Image)
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

func applyRolloutRequest(d *Deployment, req *RolloutRequest) {
	d.RolloutStrategy = string(req.Strategy)
	d.HealthCheckHost = req.HealthCheckHost
	d.TimeoutSeconds = req.TimeoutSeconds
	if d.TimeoutSeconds <= 0 {
		d.TimeoutSeconds = DefaultTimeoutSeconds
	}
	d.HealthGateEnabled = req.HealthGateEnabled
	d.HealthGateThreshold = req.HealthGateThreshold
	if d.HealthGateThreshold <= 0 {
		d.HealthGateThreshold = DefaultHealthGateThreshold
	}
	d.HealthGateIntervalMs = req.HealthGateIntervalMs
	if d.HealthGateIntervalMs <= 0 {
		d.HealthGateIntervalMs = DefaultHealthGateIntervalMs
	}
	d.AutoRollbackEnabled = req.AutoRollbackEnabled
	d.RollbackOnHealthFailure = req.RollbackOnHealthFailure
	d.CleanupOnFailure = req.CleanupOnFailure
	d.TargetReplicas = req.TargetReplicas
	if d.TargetReplicas <= 0 {
		d.TargetReplicas = 1
	}
	d.ProgressPct = 0
	d.NextStep = 0
}

func isActiveStatus(s string) bool {
	switch s {
	case string(StatusPending), string(StatusInProgress),
		string(StatusProvisioning), string(StatusAwaitingHealth), string(StatusPromoting),
		string(StatusRollbackPending), string(StatusRollingBack):
		return true
	}
	return false
}

func (s *Service) recreateRollout(ctx context.Context, req *RolloutRequest) (*Deployment, error) {
	existing, err := s.store.ListDeployments(ctx, req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("check existing: %w", err)
	}
	for _, d := range existing {
		if isActiveStatus(d.Status) {
			return nil, ErrInProgress
		}
	}

	now := time.Now().UTC()
	deployment := &Deployment{
		ID:              uuid.NewString(),
		ServerID:        req.ServerID,
		Strategy:        StrategyRecreate,
		Status:          StatusPending,
		Image:           req.Image,
		HealthCheckPath: req.HealthCheckPath,
		HealthCheckPort: req.HealthCheckPort,
		HealthCheckHost: req.HealthCheckHost,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	applyRolloutRequest(deployment, req)

	if err := s.store.CreateDeployment(ctx, toStoreDeployment(deployment)); err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	if err := s.store.UpdateDeploymentRolloutStrategy(ctx, deployment.ID, string(deployment.Strategy)); err != nil {
		return nil, fmt.Errorf("set rollout strategy: %w", err)
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("deployment_started", "deployment", "server", req.ServerID, map[string]any{
			"deploymentId": deployment.ID, "image": req.Image, "strategy": deployment.Strategy,
		}))
	}

	go func(id string, parentCtx context.Context) {
		execCtx, cancel := context.WithTimeout(parentCtx, 30*time.Minute)
		defer cancel()
		if execErr := s.ExecuteDeployment(execCtx, id); execErr != nil {
			slog.Error("execute deployment", "deploymentId", id, "error", execErr.Error())
		}
	}(deployment.ID, ctx)

	return deployment, nil
}

func (s *Service) rollingRollout(ctx context.Context, req *RolloutRequest) (*Deployment, error) {
	existing, err := s.store.ListDeployments(ctx, req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("check existing: %w", err)
	}
	for _, d := range existing {
		if isActiveStatus(d.Status) {
			return nil, ErrInProgress
		}
	}

	now := time.Now().UTC()
	deployment := &Deployment{
		ID:              uuid.NewString(),
		ServerID:        req.ServerID,
		Strategy:        StrategyRolling,
		Status:          StatusPending,
		Image:           req.Image,
		HealthCheckPath: req.HealthCheckPath,
		HealthCheckPort: req.HealthCheckPort,
		HealthCheckHost: req.HealthCheckHost,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	applyRolloutRequest(deployment, req)

	if err := s.store.CreateDeployment(ctx, toStoreDeployment(deployment)); err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	if err := s.store.UpdateDeploymentRolloutStrategy(ctx, deployment.ID, string(deployment.Strategy)); err != nil {
		return nil, fmt.Errorf("set rollout strategy: %w", err)
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("rolling_update_started", "deployment", "server", req.ServerID, map[string]any{
			"deploymentId": deployment.ID, "image": req.Image, "strategy": deployment.Strategy,
		}))
	}

	go func(id string, parentCtx context.Context) {
		execCtx, cancel := context.WithTimeout(parentCtx, 30*time.Minute)
		defer cancel()
		if execErr := s.ExecuteDeployment(execCtx, id); execErr != nil {
			slog.Error("execute deployment", "deploymentId", id, "error", execErr.Error())
		}
	}(deployment.ID, ctx)

	return deployment, nil
}

func (s *Service) blueGreenRollout(ctx context.Context, req *RolloutRequest) (*Deployment, error) {
	dep, err := s.StartBlueGreen(ctx, req.ServerID, req.Image, req.HealthCheckPath, req.HealthCheckPort)
	if err != nil {
		return nil, err
	}

	applyRolloutRequest(dep, req)
	dep.Status = StatusPending
	dep.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateDeployment(ctx, toStoreDeployment(dep)); err != nil {
		return nil, fmt.Errorf("update deployment: %w", err)
	}

	if err := s.store.UpdateDeploymentRolloutStrategy(ctx, dep.ID, string(dep.Strategy)); err != nil {
		return nil, fmt.Errorf("set rollout strategy: %w", err)
	}

	go func(id string, parentCtx context.Context) {
		execCtx, cancel := context.WithTimeout(parentCtx, 30*time.Minute)
		defer cancel()
		if execErr := s.ExecuteDeployment(execCtx, id); execErr != nil {
			slog.Error("execute deployment", "deploymentId", id, "error", execErr.Error())
		}
	}(dep.ID, ctx)

	return dep, nil
}

func (s *Service) canaryRollout(ctx context.Context, req *RolloutRequest) (*Deployment, error) {
	existing, err := s.store.ListDeployments(ctx, req.ServerID)
	if err != nil {
		return nil, fmt.Errorf("check existing: %w", err)
	}
	for _, d := range existing {
		if isActiveStatus(d.Status) {
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
		Status:          StatusPending,
		Image:           req.Image,
		BlueTargetID:    fmt.Sprintf("%s-stable", req.ServerID),
		GreenTargetID:   fmt.Sprintf("%s-canary-%d", req.ServerID, time.Now().Unix()),
		ActiveTarget:    "blue",
		HealthCheckPath: req.HealthCheckPath,
		HealthCheckPort: req.HealthCheckPort,
		HealthCheckHost: req.HealthCheckHost,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	applyRolloutRequest(deployment, req)

	if err := s.store.CreateDeployment(ctx, toStoreDeployment(deployment)); err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	if err := s.store.UpdateDeploymentRolloutStrategy(ctx, deployment.ID, string(deployment.Strategy)); err != nil {
		return nil, fmt.Errorf("set rollout strategy: %w", err)
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("canary_update_started", "deployment", "server", req.ServerID, map[string]any{
			"deploymentId": deployment.ID, "image": req.Image, "strategy": deployment.Strategy,
			"canaryPercent": canaryPct,
		}))
	}

	go func(id string, parentCtx context.Context) {
		execCtx, cancel := context.WithTimeout(parentCtx, 30*time.Minute)
		defer cancel()
		if execErr := s.ExecuteDeployment(execCtx, id); execErr != nil {
			slog.Error("execute deployment", "deploymentId", id, "error", execErr.Error())
		}
	}(deployment.ID, ctx)

	return deployment, nil
}

func validateImageRef(imageRef string) error {
	if imageRef == "" {
		return ErrInvalidImage
	}
	if !strings.Contains(imageRef, "@sha256:") {
		return ErrInvalidImageRef
	}
	return nil
}

func digestImageRef(imageRef string) string {
	idx := strings.Index(imageRef, "@sha256:")
	if idx == -1 {
		return imageRef
	}
	before := imageRef[:idx]
	if lastColon := strings.LastIndex(before, ":"); lastColon != -1 {
		before = before[:lastColon]
	}
	return before + imageRef[idx:]
}

func ValidateImageRef(imageRef string) error {
	return validateImageRef(imageRef)
}
