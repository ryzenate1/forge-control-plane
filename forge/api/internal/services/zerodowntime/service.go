package zerodowntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

var (
	ErrNotFound          = errors.New("release not found")
	ErrInProgress        = errors.New("deployment already in progress")
	ErrNoRollback        = errors.New("no previous release to rollback to")
	ErrHealthCheckFailed = errors.New("health check failed")
)

type ReleaseStatus string

const (
	StatusPending       ReleaseStatus = "pending"
	StatusBuilding      ReleaseStatus = "building"
	StatusDeploying     ReleaseStatus = "deploying"
	StatusHealthChecking ReleaseStatus = "health_checking"
	StatusLive          ReleaseStatus = "live"
	StatusRolledBack    ReleaseStatus = "rolled_back"
	StatusFailed        ReleaseStatus = "failed"
)

type Release struct {
	ID          string     `json:"id"`
	ServerID    string     `json:"serverId"`
	Version     int        `json:"version"`
	ImageTag    string     `json:"imageTag"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type HealthCheckResult struct {
	ID             string    `json:"id"`
	DeploymentID   string    `json:"deploymentId"`
	CheckTimestamp time.Time `json:"checkTimestamp"`
	Status         string    `json:"status"`
	ResponseCode   int       `json:"responseCode"`
	ResponseTimeMs int       `json:"responseTimeMs"`
	ErrorMessage   string    `json:"errorMessage"`
}

type HealthCheckConfig struct {
	Path               string `json:"path"`
	Port               int    `json:"port"`
	Protocol           string `json:"protocol"`
	IntervalSeconds    int    `json:"intervalSeconds"`
	TimeoutSeconds     int    `json:"timeoutSeconds"`
	HealthyThreshold   int    `json:"healthyThreshold"`
	UnhealthyThreshold int    `json:"unhealthyThreshold"`
}

type DeploymentEvent struct {
	ID        string    `json:"id"`
	EventType string    `json:"eventType"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

type Service struct {
	store *store.Store
}

func New(store *store.Store) *Service {
	return &Service{store: store}
}

func toServiceRelease(r store.DeploymentRelease) *Release {
	return &Release{
		ID:          r.ID,
		ServerID:    r.ServerID,
		Version:     r.Version,
		ImageTag:    r.ImageTag,
		Status:      r.Status,
		CreatedAt:   r.CreatedAt,
		CompletedAt: r.CompletedAt,
	}
}

func (s *Service) CreateRelease(ctx context.Context, serverID, imageTag string) (*Release, error) {
	latest, err := s.store.GetLatestDeploymentRelease(ctx, serverID)
	nextVersion := 1
	if err == nil {
		nextVersion = latest.Version + 1
	}

	now := time.Now().UTC()
	release := &store.DeploymentRelease{
		ID:        uuid.NewString(),
		ServerID:  serverID,
		Version:   nextVersion,
		ImageTag:  imageTag,
		Status:    string(StatusPending),
		CreatedAt: now,
	}

	if err := s.store.CreateDeploymentRelease(ctx, release); err != nil {
		return nil, fmt.Errorf("create release: %w", err)
	}

	s.appendEvent(ctx, release.ID, "release_created", fmt.Sprintf("Release v%d created with image %s", nextVersion, imageTag))

	return toServiceRelease(*release), nil
}

func (s *Service) DeployRelease(ctx context.Context, releaseID string) (*Release, error) {
	r, err := s.store.GetDeploymentRelease(ctx, releaseID)
	if err != nil {
		return nil, ErrNotFound
	}

	if r.Status != string(StatusPending) && r.Status != string(StatusFailed) && r.Status != string(StatusRolledBack) {
		return nil, ErrInProgress
	}

	if err := s.store.UpdateDeploymentReleaseStatus(ctx, releaseID, string(StatusDeploying), nil); err != nil {
		return nil, fmt.Errorf("update release status: %w", err)
	}

	s.appendEvent(ctx, releaseID, "deploying", fmt.Sprintf("Deploying release v%d", r.Version))

	r.Status = string(StatusDeploying)
	return toServiceRelease(r), nil
}

func (s *Service) RunHealthChecks(ctx context.Context, releaseID string) (bool, error) {
	r, err := s.store.GetDeploymentRelease(ctx, releaseID)
	if err != nil {
		return false, ErrNotFound
	}

	config, err := s.store.GetZeroDowntimeHealthCheckConfig(ctx, r.ServerID)
	if err != nil {
		s.appendEvent(ctx, releaseID, "health_skipped", "No health check config found, skipping")
		return true, nil
	}

	if err := s.store.UpdateDeploymentReleaseStatus(ctx, releaseID, string(StatusHealthChecking), nil); err != nil {
		return false, fmt.Errorf("update release status: %w", err)
	}

	s.appendEvent(ctx, releaseID, "health_check_started", "Running health checks")

	interval := time.Duration(config.IntervalSeconds) * time.Second
	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	healthyThreshold := config.HealthyThreshold
	unhealthyThreshold := config.UnhealthyThreshold

	if interval <= 0 {
		interval = 10 * time.Second
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if healthyThreshold <= 0 {
		healthyThreshold = 2
	}
	if unhealthyThreshold <= 0 {
		unhealthyThreshold = 3
	}

	// Allow up to 2 minutes for all health checks to complete
	maxDuration := 2 * time.Minute
	healthCtx, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()

	healthyCount := 0
	unhealthyCount := 0
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-healthCtx.Done():
			s.appendEvent(ctx, releaseID, "health_check_failed",
				fmt.Sprintf("Health checks timed out (healthy: %d, unhealthy: %d)", healthyCount, unhealthyCount))

			_ = s.store.UpdateDeploymentReleaseStatus(ctx, releaseID, string(StatusFailed), nil)
			return false, ErrHealthCheckFailed

		case <-ticker.C:
			start := time.Now()
			result, err := s.doHealthCheck(healthCtx, r.ServerID, config)
			elapsed := time.Since(start)

			hcResult := store.ZeroDowntimeHealthCheckResult{
				ID:              uuid.NewString(),
				DeploymentID:    releaseID,
				CheckTimestamp:  time.Now().UTC(),
				ResponseTimeMs:  int(elapsed.Milliseconds()),
			}

			if err != nil {
				hcResult.Status = "error"
				hcResult.ErrorMessage = err.Error()
				healthyCount = 0
				unhealthyCount++
			} else if result >= 200 && result < 400 {
				hcResult.Status = "healthy"
				hcResult.ResponseCode = result
				healthyCount++
				unhealthyCount = 0
			} else {
				hcResult.Status = "unhealthy"
				hcResult.ResponseCode = result
				healthyCount = 0
				unhealthyCount++
			}

			_ = s.store.CreateZeroDowntimeHealthCheckResult(ctx, &hcResult)

			if healthyCount >= healthyThreshold {
				s.appendEvent(ctx, releaseID, "health_check_passed",
					fmt.Sprintf("Health checks passed (%d consecutive successes)", healthyCount))
				return true, nil
			}

			if unhealthyCount >= unhealthyThreshold {
				s.appendEvent(ctx, releaseID, "health_check_failed",
					fmt.Sprintf("Health checks failed (%d consecutive failures)", unhealthyCount))

				_ = s.store.UpdateDeploymentReleaseStatus(ctx, releaseID, string(StatusFailed), nil)
				return false, ErrHealthCheckFailed
			}
		}
	}
}

func (s *Service) doHealthCheck(ctx context.Context, serverID string, config store.ZeroDowntimeHealthCheckConfig) (int, error) {
	protocol := config.Protocol
	if protocol == "" {
		protocol = "http"
	}
	path := config.Path
	if path == "" {
		path = "/health"
	}
	port := config.Port
	if port <= 0 {
		port = 80
	}

	url := fmt.Sprintf("%s://localhost:%d%s", protocol, port, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	client := &http.Client{Timeout: time.Duration(config.TimeoutSeconds) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}

func (s *Service) PromoteRelease(ctx context.Context, releaseID string) (*Release, error) {
	r, err := s.store.GetDeploymentRelease(ctx, releaseID)
	if err != nil {
		return nil, ErrNotFound
	}

	if r.Status != string(StatusHealthChecking) && r.Status != string(StatusDeploying) {
		return nil, fmt.Errorf("release must be in deploying or health_checking status to promote")
	}

	now := time.Now().UTC()
	if err := s.store.UpdateDeploymentReleaseStatus(ctx, releaseID, string(StatusLive), &now); err != nil {
		return nil, fmt.Errorf("update release status: %w", err)
	}

	s.appendEvent(ctx, releaseID, "promoted", fmt.Sprintf("Release v%d promoted to live", r.Version))

	r.Status = string(StatusLive)
	r.CompletedAt = &now
	return toServiceRelease(r), nil
}

func (s *Service) RollbackRelease(ctx context.Context, releaseID string) (*Release, error) {
	r, err := s.store.GetDeploymentRelease(ctx, releaseID)
	if err != nil {
		return nil, ErrNotFound
	}

	if r.Status != string(StatusLive) && r.Status != string(StatusFailed) {
		return nil, fmt.Errorf("can only rollback a live or failed release")
	}

	prevRelease, err := s.getPreviousLiveRelease(ctx, r.ServerID, r.Version)
	if err != nil {
		return nil, ErrNoRollback
	}

	now := time.Now().UTC()
	if err := s.store.UpdateDeploymentReleaseStatus(ctx, releaseID, string(StatusRolledBack), &now); err != nil {
		return nil, fmt.Errorf("rollback release: %w", err)
	}

	if err := s.store.UpdateDeploymentReleaseStatus(ctx, prevRelease.ID, string(StatusLive), nil); err != nil {
		return nil, fmt.Errorf("re-promote previous release: %w", err)
	}

	s.appendEvent(ctx, releaseID, "rolled_back", fmt.Sprintf("Release v%d rolled back to v%d", r.Version, prevRelease.Version))
	s.appendEvent(ctx, prevRelease.ID, "promoted", fmt.Sprintf("Release v%d re-promoted after rollback", prevRelease.Version))

	return toServiceRelease(r), nil
}

func (s *Service) getPreviousLiveRelease(ctx context.Context, serverID string, currentVersion int) (*store.DeploymentRelease, error) {
	releases, err := s.store.ListDeploymentReleases(ctx, serverID)
	if err != nil {
		return nil, err
	}

	for _, r := range releases {
		if r.Version < currentVersion {
			return &r, nil
		}
	}
	return nil, ErrNoRollback
}

func (s *Service) CleanupOldReleases(ctx context.Context, serverID string, keep int) error {
	releases, err := s.store.ListDeploymentReleases(ctx, serverID)
	if err != nil {
		return err
	}

	if keep <= 0 {
		keep = 5
	}

	if len(releases) <= keep {
		return nil
	}

	for i := keep; i < len(releases); i++ {
		r := releases[i]
		if r.Status != string(StatusLive) {
			slog.Info("cleaning up old release", "serverId", serverID, "releaseId", r.ID, "version", r.Version)
		}
	}

	return nil
}

func (s *Service) ListReleases(ctx context.Context, serverID string) ([]*Release, error) {
	releases, err := s.store.ListDeploymentReleases(ctx, serverID)
	if err != nil {
		return nil, err
	}

	result := make([]*Release, 0, len(releases))
	for i := range releases {
		result = append(result, toServiceRelease(releases[i]))
	}
	return result, nil
}

func (s *Service) GetRelease(ctx context.Context, releaseID string) (*Release, error) {
	r, err := s.store.GetDeploymentRelease(ctx, releaseID)
	if err != nil {
		return nil, ErrNotFound
	}
	return toServiceRelease(r), nil
}

func (s *Service) GetHealthCheckConfig(ctx context.Context, serverID string) (*HealthCheckConfig, error) {
	c, err := s.store.GetZeroDowntimeHealthCheckConfig(ctx, serverID)
	if err != nil {
		return nil, err
	}
	return &HealthCheckConfig{
		Path:               c.Path,
		Port:               c.Port,
		Protocol:           c.Protocol,
		IntervalSeconds:    c.IntervalSeconds,
		TimeoutSeconds:     c.TimeoutSeconds,
		HealthyThreshold:   c.HealthyThreshold,
		UnhealthyThreshold: c.UnhealthyThreshold,
	}, nil
}

func (s *Service) UpsertHealthCheckConfig(ctx context.Context, serverID string, cfg *HealthCheckConfig) (*HealthCheckConfig, error) {
	now := time.Now().UTC()
	storeCfg := &store.ZeroDowntimeHealthCheckConfig{
		ID:                 uuid.NewString(),
		ServerID:           serverID,
		Path:               cfg.Path,
		Port:               cfg.Port,
		Protocol:           cfg.Protocol,
		IntervalSeconds:    cfg.IntervalSeconds,
		TimeoutSeconds:     cfg.TimeoutSeconds,
		HealthyThreshold:   cfg.HealthyThreshold,
		UnhealthyThreshold: cfg.UnhealthyThreshold,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.store.UpsertZeroDowntimeHealthCheckConfig(ctx, storeCfg); err != nil {
		return nil, err
	}

	return s.GetHealthCheckConfig(ctx, serverID)
}

func (s *Service) GetHealthCheckResults(ctx context.Context, deploymentID string) ([]*HealthCheckResult, error) {
	results, err := s.store.ListZeroDowntimeHealthCheckResults(ctx, deploymentID)
	if err != nil {
		return nil, err
	}

	svcResults := make([]*HealthCheckResult, 0, len(results))
	for i := range results {
		svcResults = append(svcResults, &HealthCheckResult{
			ID:             results[i].ID,
			DeploymentID:   results[i].DeploymentID,
			CheckTimestamp: results[i].CheckTimestamp,
			Status:         results[i].Status,
			ResponseCode:   results[i].ResponseCode,
			ResponseTimeMs: results[i].ResponseTimeMs,
			ErrorMessage:   results[i].ErrorMessage,
		})
	}
	return svcResults, nil
}

func (s *Service) GetDeploymentEvents(ctx context.Context, deploymentID string) ([]*DeploymentEvent, error) {
	events, err := s.store.ListDeploymentEvents(ctx, deploymentID)
	if err != nil {
		return nil, err
	}

	svcEvents := make([]*DeploymentEvent, 0, len(events))
	for i := range events {
		svcEvents = append(svcEvents, &DeploymentEvent{
			ID:        events[i].ID,
			EventType: events[i].EventType,
			Message:   events[i].Message,
			CreatedAt: events[i].CreatedAt,
		})
	}
	return svcEvents, nil
}

func (s *Service) GetActiveRelease(ctx context.Context, serverID string) (*Release, error) {
	r, err := s.store.GetActiveDeploymentRelease(ctx, serverID)
	if err != nil {
		return nil, ErrNotFound
	}
	return toServiceRelease(r), nil
}

func (s *Service) appendEvent(ctx context.Context, deploymentID, eventType, message string) {
	if err := s.store.CreateDeploymentEvent(ctx, &store.DeploymentEvent{
		ID:           uuid.NewString(),
		DeploymentID: deploymentID,
		EventType:    eventType,
		Message:      message,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		slog.Error("append deployment event", "deploymentId", deploymentID, "error", err.Error())
	}
}
