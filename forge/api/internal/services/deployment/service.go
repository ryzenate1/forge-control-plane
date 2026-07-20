package deployment

import (
	"context"
	"errors"
	"fmt"
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
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusRolledBack Status = "rolled_back"
	StatusCancelled  Status = "cancelled"
)

type Deployment struct {
	ID                 string     `json:"id"`
	ServerID           string     `json:"serverId"`
	Strategy           Strategy   `json:"strategy"`
	Status             Status     `json:"status"`
	Image              string     `json:"image"`
	BlueTargetID       string     `json:"blueTargetId"`
	GreenTargetID      string     `json:"greenTargetId"`
	ActiveTarget       string     `json:"activeTarget"`
	HealthCheckPath    string     `json:"healthCheckPath,omitempty"`
	HealthCheckPort    int        `json:"healthCheckPort,omitempty"`
	CurrentRevisionID  *string    `json:"currentRevisionId,omitempty"`
	RolloutStrategy    string     `json:"rolloutStrategy,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`
	Error              string     `json:"error,omitempty"`
}

type Service struct {
	store     *store.Store
	publisher events.Publisher
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
	ErrNotFound      = errors.New("deployment not found")
	ErrInProgress    = errors.New("deployment already in progress for this server")
	ErrInvalidImage  = errors.New("image is required")
	ErrInvalidServer = errors.New("serverId is required")
	ErrNoRollback    = errors.New("no rollback target available")
)

func toServiceDeployment(d store.Deployment) *Deployment {
	return &Deployment{
		ID:                d.ID,
		ServerID:          d.ServerID,
		Strategy:          Strategy(d.Strategy),
		Status:            Status(d.Status),
		Image:             d.Image,
		BlueTargetID:      d.BlueTargetID,
		GreenTargetID:     d.GreenTargetID,
		ActiveTarget:      d.ActiveTarget,
		HealthCheckPath:   d.HealthCheckPath,
		HealthCheckPort:   d.HealthCheckPort,
		Error:             d.Error,
		CurrentRevisionID: d.CurrentRevisionID,
		RolloutStrategy:   d.RolloutStrategy,
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
		CompletedAt:       d.CompletedAt,
	}
}

func toStoreDeployment(d *Deployment) *store.Deployment {
	return &store.Deployment{
		ID:                d.ID,
		ServerID:          d.ServerID,
		Strategy:          string(d.Strategy),
		Status:            string(d.Status),
		Image:             d.Image,
		BlueTargetID:      d.BlueTargetID,
		GreenTargetID:     d.GreenTargetID,
		ActiveTarget:      d.ActiveTarget,
		HealthCheckPath:   d.HealthCheckPath,
		HealthCheckPort:   d.HealthCheckPort,
		Error:             d.Error,
		CurrentRevisionID: d.CurrentRevisionID,
		RolloutStrategy:   d.RolloutStrategy,
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
		CompletedAt:       d.CompletedAt,
	}
}

func (s *Service) StartBlueGreen(ctx context.Context, serverID, newImage string, healthCheckPath string, healthCheckPort int) (*Deployment, error) {
	if serverID == "" {
		return nil, ErrInvalidServer
	}
	if newImage == "" {
		return nil, ErrInvalidImage
	}

	existing, err := s.store.ListDeployments(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("check existing deployments: %w", err)
	}
	for _, d := range existing {
		if d.Status == string(StatusInProgress) {
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
	if deployment.ActiveTarget == "green" {
		deployment.ActiveTarget = "blue"
	}

	now := time.Now().UTC()
	deployment.Status = StatusRolledBack
	deployment.UpdatedAt = now
	deployment.CompletedAt = &now

	if err := s.store.UpdateDeployment(ctx, toStoreDeployment(deployment)); err != nil {
		return nil, fmt.Errorf("update deployment: %w", err)
	}

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("deployment_rolled_back", "deployment", "deployment", deploymentID, map[string]any{
			"serverId": deployment.ServerID, "from": oldTarget, "to": deployment.ActiveTarget,
		}))
	}

	return deployment, nil
}

func (s *Service) CompleteDeployment(ctx context.Context, deploymentID string) (*Deployment, error) {
	sd, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, ErrNotFound
	}

	deployment := toServiceDeployment(sd)
	now := time.Now().UTC()
	deployment.Status = StatusCompleted
	deployment.UpdatedAt = now
	deployment.CompletedAt = &now

	if err := s.store.UpdateDeployment(ctx, toStoreDeployment(deployment)); err != nil {
		return nil, fmt.Errorf("update deployment: %w", err)
	}

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
	if deployment.Status != StatusInProgress && deployment.Status != StatusPending {
		return nil, errors.New("can only cancel pending or in-progress deployments")
	}

	now := time.Now().UTC()
	deployment.Status = StatusCancelled
	deployment.UpdatedAt = now
	deployment.CompletedAt = &now

	if err := s.store.UpdateDeployment(ctx, toStoreDeployment(deployment)); err != nil {
		return nil, fmt.Errorf("update deployment: %w", err)
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
	sds, err := s.store.ListDeployments(ctx, serverID)
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

	deployment := toServiceDeployment(sd)
	deployment.Status = status
	deployment.UpdatedAt = time.Now().UTC()
	if errMsg != "" {
		deployment.Error = errMsg
	}
	if status == StatusCompleted || status == StatusFailed || status == StatusRolledBack {
		now := time.Now().UTC()
		deployment.CompletedAt = &now
	}

	return s.store.UpdateDeployment(ctx, toStoreDeployment(deployment))
}
