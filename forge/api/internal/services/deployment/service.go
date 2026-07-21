package deployment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type Strategy string

const (
	StrategyBlueGreen Strategy = "blue-green"
	StrategyCanary    Strategy = "canary"
	StrategyRolling   Strategy = "rolling"
	StrategyRecreate  Strategy = "recreate"
)

type Status string

const (
	StatusPending       Status = "pending"
	StatusProvisioning  Status = "provisioning"
	StatusInProgress    Status = "in_progress"
	StatusAwaitingHealth Status = "awaiting_health"
	StatusPromoting     Status = "promoting"
	StatusCompleted     Status = "completed"
	StatusFailed        Status = "failed"
	StatusRollbackPending Status = "rollback_pending"
	StatusRollingBack   Status = "rolling_back"
	StatusRolledBack    Status = "rolled_back"
	StatusCancelled     Status = "cancelled"
)

type StepStatus string

const (
	StepStatusPending    StepStatus = "pending"
	StepStatusRunning    StepStatus = "in_progress"
	StepStatusCompleted  StepStatus = "completed"
	StepStatusFailed     StepStatus = "failed"
	StepStatusCancelled  StepStatus = "cancelled"
	StepStatusSkipped    StepStatus = "skipped"
)

const (
	DefaultTimeoutSeconds       = 300
	DefaultHealthGateThreshold  = 3
	DefaultHealthGateIntervalMs = 5000
)

type Deployment struct {
	ID                      string     `json:"id"`
	ServerID                string     `json:"serverId"`
	Strategy                Strategy   `json:"strategy"`
	Status                  Status     `json:"status"`
	Image                   string     `json:"image"`
	BlueTargetID            string     `json:"blueTargetId"`
	GreenTargetID           string     `json:"greenTargetId"`
	ActiveTarget            string     `json:"activeTarget"`
	HealthCheckPath         string     `json:"healthCheckPath,omitempty"`
	HealthCheckPort         int        `json:"healthCheckPort,omitempty"`
	HealthCheckHost         string     `json:"healthCheckHost,omitempty"`
	CurrentRevisionID       *string    `json:"currentRevisionId,omitempty"`
	RolloutStrategy         string     `json:"rolloutStrategy,omitempty"`
	TimeoutSeconds          int        `json:"timeoutSeconds,omitempty"`
	HealthGateEnabled       bool       `json:"healthGateEnabled,omitempty"`
	HealthGateThreshold     int        `json:"healthGateThreshold,omitempty"`
	HealthGateIntervalMs    int        `json:"healthGateIntervalMs,omitempty"`
	AutoRollbackEnabled     bool       `json:"autoRollbackEnabled,omitempty"`
	RollbackOnHealthFailure bool       `json:"rollbackOnHealthFailure,omitempty"`
	CleanupOnFailure        bool       `json:"cleanupOnFailure,omitempty"`
	TargetReplicas          int        `json:"targetReplicas,omitempty"`
	ProgressPct             int        `json:"progressPct,omitempty"`
	NextStep                int        `json:"nextStep,omitempty"`
	TimeoutAt               *time.Time `json:"timeoutAt,omitempty"`
	ExecutorID              string     `json:"executorId,omitempty"`
	ExecutionLeaseUntil     *time.Time `json:"executionLeaseUntil,omitempty"`
	Version                 int        `json:"version"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
	CompletedAt             *time.Time `json:"completedAt,omitempty"`
	Error                   string     `json:"error,omitempty"`
}

type DeploymentStep struct {
	ID           string     `json:"id"`
	DeploymentID string     `json:"deploymentId"`
	StepNumber   int        `json:"stepNumber"`
	StepName     string     `json:"stepName"`
	Status       StepStatus `json:"status"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type HealthCheckResult struct {
	Passed bool   `json:"passed"`
	Status int    `json:"status"`
	Body   string `json:"body,omitempty"`
	Error  string `json:"error,omitempty"`
}

type Service struct {
	store                *store.Store
	publisher            events.Publisher
	resumeMu             sync.Mutex
	executingDeployments sync.Map
}

func New(store *store.Store, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{
		store:     store,
		publisher: publisher,
	}
}

var (
	ErrNotFound           = errors.New("deployment not found")
	ErrInProgress         = errors.New("deployment already in progress for this server")
	ErrInvalidImage       = errors.New("image is required")
	ErrInvalidImageRef    = errors.New("image reference must contain a digest (@sha256:)")
	ErrInvalidServer      = errors.New("serverId is required")
	ErrNoRollback         = errors.New("no rollback target available")
	ErrNotTerminal        = errors.New("deployment is not in a terminal state")
	ErrHealthCheckFailed  = errors.New("health check failed")
	ErrStepNotFound       = errors.New("deployment step not found")
	ErrDeploymentTimedOut = errors.New("deployment timed out")
	ErrAlreadyCancelled   = errors.New("deployment already cancelled")
	ErrCleanupFailed      = errors.New("cleanup of previous deployment failed")
)

func toServiceDeployment(d store.Deployment) *Deployment {
	return &Deployment{
		ID:                      d.ID,
		ServerID:                d.ServerID,
		Strategy:                Strategy(d.Strategy),
		Status:                  Status(d.Status),
		Image:                   d.Image,
		BlueTargetID:            d.BlueTargetID,
		GreenTargetID:           d.GreenTargetID,
		ActiveTarget:            d.ActiveTarget,
		HealthCheckPath:         d.HealthCheckPath,
		HealthCheckPort:         d.HealthCheckPort,
		HealthCheckHost:         d.HealthCheckHost,
		Error:                   d.Error,
		CurrentRevisionID:       d.CurrentRevisionID,
		RolloutStrategy:         d.RolloutStrategy,
		TimeoutSeconds:          d.TimeoutSeconds,
		HealthGateEnabled:       d.HealthGateEnabled,
		HealthGateThreshold:     d.HealthGateThreshold,
		HealthGateIntervalMs:    d.HealthGateIntervalMs,
		AutoRollbackEnabled:     d.AutoRollbackEnabled,
		RollbackOnHealthFailure: d.RollbackOnHealthFailure,
		CleanupOnFailure:        d.CleanupOnFailure,
		TargetReplicas:          d.TargetReplicas,
		ProgressPct:             d.ProgressPct,
		NextStep:                d.NextStep,
		TimeoutAt:               d.TimeoutAt,
		ExecutorID:              d.ExecutorID,
		ExecutionLeaseUntil:     d.ExecutionLeaseUntil,
		Version:                 d.Version,
		CreatedAt:               d.CreatedAt,
		UpdatedAt:               d.UpdatedAt,
		CompletedAt:             d.CompletedAt,
	}
}

func toStoreDeployment(d *Deployment) *store.Deployment {
	return &store.Deployment{
		ID:                      d.ID,
		ServerID:                d.ServerID,
		Strategy:                string(d.Strategy),
		Status:                  string(d.Status),
		Image:                   d.Image,
		BlueTargetID:            d.BlueTargetID,
		GreenTargetID:           d.GreenTargetID,
		ActiveTarget:            d.ActiveTarget,
		HealthCheckPath:         d.HealthCheckPath,
		HealthCheckPort:         d.HealthCheckPort,
		HealthCheckHost:         d.HealthCheckHost,
		Error:                   d.Error,
		CurrentRevisionID:       d.CurrentRevisionID,
		RolloutStrategy:         d.RolloutStrategy,
		TimeoutSeconds:          d.TimeoutSeconds,
		HealthGateEnabled:       d.HealthGateEnabled,
		HealthGateThreshold:     d.HealthGateThreshold,
		HealthGateIntervalMs:    d.HealthGateIntervalMs,
		AutoRollbackEnabled:     d.AutoRollbackEnabled,
		RollbackOnHealthFailure: d.RollbackOnHealthFailure,
		CleanupOnFailure:        d.CleanupOnFailure,
		TargetReplicas:          d.TargetReplicas,
		ProgressPct:             d.ProgressPct,
		NextStep:                d.NextStep,
		TimeoutAt:               d.TimeoutAt,
		ExecutorID:              d.ExecutorID,
		ExecutionLeaseUntil:     d.ExecutionLeaseUntil,
		Version:                 d.Version,
		CreatedAt:               d.CreatedAt,
		UpdatedAt:               d.UpdatedAt,
		CompletedAt:             d.CompletedAt,
	}
}

func toServiceStep(s store.DeploymentStep) *DeploymentStep {
	return &DeploymentStep{
		ID:           s.ID,
		DeploymentID: s.DeploymentID,
		StepNumber:   s.StepNumber,
		StepName:     s.StepName,
		Status:       StepStatus(s.Status),
		StartedAt:    s.StartedAt,
		CompletedAt:  s.CompletedAt,
		Error:        s.Error,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
	}
}

func toStoreStep(s *DeploymentStep) *store.DeploymentStep {
	return &store.DeploymentStep{
		ID:           s.ID,
		DeploymentID: s.DeploymentID,
		StepNumber:   s.StepNumber,
		StepName:     s.StepName,
		Status:       string(s.Status),
		StartedAt:    s.StartedAt,
		CompletedAt:  s.CompletedAt,
		Error:        s.Error,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
	}
}

func (s *Service) StartBlueGreen(ctx context.Context, serverID, newImage string, healthCheckPath string, healthCheckPort int) (*Deployment, error) {
	if serverID == "" {
		return nil, ErrInvalidServer
	}
	if err := validateImageRef(newImage); err != nil {
		return nil, err
	}
	newImage = digestImageRef(newImage)

	existing, err := s.store.ListDeployments(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("check existing deployments: %w", err)
	}
	for _, d := range existing {
		if d.Status == string(StatusInProgress) || d.Status == string(StatusPending) ||
			d.Status == string(StatusProvisioning) || d.Status == string(StatusAwaitingHealth) ||
			d.Status == string(StatusPromoting) || d.Status == string(StatusRollbackPending) ||
			d.Status == string(StatusRollingBack) {
			return nil, ErrInProgress
		}
	}

	now := time.Now().UTC()
	deployment := &Deployment{
		ID:              uuid.NewString(),
		ServerID:        serverID,
		Strategy:        StrategyBlueGreen,
		Status:          StatusPending,
		Image:           newImage,
		BlueTargetID:    fmt.Sprintf("%s-blue", serverID),
		GreenTargetID:   fmt.Sprintf("%s-green-%d", serverID, time.Now().Unix()),
		ActiveTarget:    "blue",
		HealthCheckPath: healthCheckPath,
		HealthCheckPort: healthCheckPort,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.store.CreateDeployment(ctx, toStoreDeployment(deployment)); err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("deployment_started", "deployment", "server", serverID, map[string]any{
			"deploymentId": deployment.ID, "image": newImage, "strategy": deployment.Strategy,
		}))
	}

	return deployment, nil
}

func (s *Service) CompleteDeployment(ctx context.Context, deploymentID string) (*Deployment, error) {
	sd, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, ErrNotFound
	}

	if err := s.store.UpdateDeploymentCompletion(ctx, deploymentID, sd.Version); err != nil {
		return nil, fmt.Errorf("update deployment: %w", err)
	}

	deployment := toServiceDeployment(sd)
	now := time.Now().UTC()
	deployment.Status = StatusCompleted
	deployment.UpdatedAt = now
	deployment.CompletedAt = &now
	deployment.ProgressPct = 100

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("deployment_completed", "deployment", "deployment", deploymentID, map[string]any{
			"serverId": deployment.ServerID, "image": deployment.Image,
		}))
	}

	return deployment, nil
}

func (s *Service) CancelDeployment(ctx context.Context, deploymentID string) (*Deployment, error) {
	sd, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, ErrNotFound
	}

	deployment := toServiceDeployment(sd)
	if deployment.Status == StatusCancelled {
		return nil, ErrAlreadyCancelled
	}
	if deployment.Status != StatusInProgress && deployment.Status != StatusPending &&
		deployment.Status != StatusProvisioning && deployment.Status != StatusAwaitingHealth &&
		deployment.Status != StatusPromoting && deployment.Status != StatusRollbackPending &&
		deployment.Status != StatusRollingBack {
		return nil, errors.New("can only cancel pending or in-progress deployments")
	}

	if err := s.store.UpdateDeploymentCancelled(ctx, deploymentID, sd.Version, ""); err != nil {
		return nil, fmt.Errorf("update deployment: %w", err)
	}

	now := time.Now().UTC()
	deployment.Status = StatusCancelled
	deployment.UpdatedAt = now
	deployment.CompletedAt = &now

	steps, err := s.store.ListDeploymentSteps(ctx, deploymentID)
	if err != nil {
		slog.Error("cancel deployment: list steps", "deploymentId", deploymentID, "error", err.Error())
	} else {
		for _, step := range steps {
			if step.Status == string(StepStatusPending) || step.Status == string(StepStatusRunning) {
				_ = s.store.UpdateDeploymentStepStatus(ctx, step.ID, string(StepStatusCancelled), "deployment cancelled")
			}
		}
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("deployment_cancelled", "deployment", "deployment", deploymentID, map[string]any{
			"serverId": deployment.ServerID,
		}))
	}

	return deployment, nil
}

func (s *Service) GetDeployment(ctx context.Context, deploymentID string) (*Deployment, error) {
	sd, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, ErrNotFound
	}
	return toServiceDeployment(sd), nil
}

func (s *Service) ListDeployments(ctx context.Context, serverID string) ([]*Deployment, error) {
	var sds []store.Deployment
	var err error
	if serverID == "" {
		sds, err = s.store.ListAllDeployments(ctx)
	} else {
		sds, err = s.store.ListDeployments(ctx, serverID)
	}
	if err != nil {
		return nil, err
	}

	result := make([]*Deployment, 0, len(sds))
	for _, d := range sds {
		result = append(result, toServiceDeployment(d))
	}
	return result, nil
}

func (s *Service) SetDeploymentStatus(ctx context.Context, deploymentID string, status Status, errMsg string) error {
	sd, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return ErrNotFound
	}

	return s.store.UpdateDeploymentStatusVersioned(ctx, deploymentID, sd.Version, string(status), errMsg)
}

func (s *Service) Rollback(ctx context.Context, deploymentID string) (*Deployment, error) {
	sd, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, ErrNotFound
	}

	deployment := toServiceDeployment(sd)
	if deployment.ActiveTarget == "blue" {
		return nil, ErrNoRollback
	}

	oldTarget := deployment.ActiveTarget

	if err := s.store.UpdateDeploymentRollback(ctx, deploymentID, sd.Version, "blue"); err != nil {
		return nil, fmt.Errorf("update deployment: %w", err)
	}

	now := time.Now().UTC()
	deployment.ActiveTarget = "blue"
	deployment.Status = StatusRolledBack
	deployment.UpdatedAt = now
	deployment.CompletedAt = &now

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("deployment_rolled_back", "deployment", "deployment", deploymentID, map[string]any{
			"serverId": deployment.ServerID, "from": oldTarget, "to": deployment.ActiveTarget,
		}))
	}

	return deployment, nil
}

func (s *Service) CleanupDeployment(ctx context.Context, deploymentID string) error {
	sd, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return ErrNotFound
	}

	deployment := toServiceDeployment(sd)
	if deployment.Status != StatusFailed && deployment.Status != StatusCancelled && deployment.Status != StatusRolledBack {
		return ErrNotTerminal
	}

	return s.store.UpdateDeploymentCancelled(ctx, deploymentID, sd.Version, "cleaned up after "+string(sd.Status))
}
