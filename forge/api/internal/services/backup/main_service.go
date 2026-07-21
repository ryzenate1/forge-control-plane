package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"gamepanel/forge/internal/store"
	"io"
	"time"

	"github.com/google/uuid"
)

// MainService is the main backup service that coordinates all backup operations
// It provides a unified interface for the API layer to interact with the backup system
type MainService struct {
	// Core services
	configService   *ConfigService
	jobService      *JobService
	artifactService *ArtifactService
	restoreService  *RestoreService

	// Dependencies
	store        *store.Store
	logger       Logger
	beaconClient BeaconClient
	scheduler    Scheduler

	// Storage adapters
	storageAdapters map[string]StorageAdapter
	defaultAdapter  StorageAdapter
}

// NewMainService creates a new MainService
func NewMainService(store *store.Store, logger Logger) *MainService {
	service := &MainService{
		store:           store,
		logger:          logger,
		storageAdapters: make(map[string]StorageAdapter),
	}

	// Initialize services
	service.configService = NewConfigService(store, logger, nil)
	service.jobService = NewJobService(store, logger)
	service.artifactService = NewArtifactService(store, logger)
	service.restoreService = NewRestoreService(store, logger)

	// Set up service dependencies
	service.jobService.SetConfigService(service.configService)
	service.jobService.SetArtifactService(service.artifactService)
	service.jobService.SetRestoreService(service.restoreService)

	service.restoreService.SetArtifactService(service.artifactService)
	service.restoreService.SetJobService(service.jobService)

	return service
}

// SetBeaconClient sets the beacon client for all services
func (s *MainService) SetBeaconClient(beaconClient BeaconClient) {
	s.beaconClient = beaconClient
	s.jobService.SetBeaconClient(beaconClient)
	s.restoreService.SetBeaconClient(beaconClient)
}

// SetScheduler sets the scheduler for all services
func (s *MainService) SetScheduler(scheduler Scheduler) {
	s.scheduler = scheduler
	s.configService.scheduler = scheduler
	s.jobService.SetScheduler(scheduler)
	s.restoreService.SetScheduler(scheduler)
}

// RegisterStorageAdapter registers a storage adapter
func (s *MainService) RegisterStorageAdapter(provider string, adapter StorageAdapter) {
	s.storageAdapters[provider] = adapter
	s.artifactService.RegisterStorageAdapter(provider, adapter)

	// Set as default if it's the first one
	if s.defaultAdapter == nil {
		s.defaultAdapter = adapter
		s.artifactService.SetDefaultAdapter(adapter)
	}
}

// =============================================================================
// Backup Configuration API
// =============================================================================

// CreateBackupConfig creates a new backup configuration
func (s *MainService) CreateBackupConfig(ctx context.Context, req CreateBackupConfigRequest, userID string) (*BackupConfig, error) {
	return s.configService.Create(ctx, req, userID)
}

// GetBackupConfig retrieves a backup configuration by ID
func (s *MainService) GetBackupConfig(ctx context.Context, configID string) (*BackupConfig, error) {
	return s.configService.Get(ctx, configID)
}

// ListBackupConfigs lists backup configurations with optional filtering
func (s *MainService) ListBackupConfigs(ctx context.Context, filters BackupConfigFilter) ([]*BackupConfig, int, error) {
	return s.configService.List(ctx, filters)
}

// UpdateBackupConfig updates a backup configuration
func (s *MainService) UpdateBackupConfig(ctx context.Context, configID string, req UpdateBackupConfigRequest, userID string) (*BackupConfig, error) {
	return s.configService.Update(ctx, configID, req, userID)
}

// DeleteBackupConfig deletes a backup configuration
func (s *MainService) DeleteBackupConfig(ctx context.Context, configID string, userID string) error {
	return s.configService.Delete(ctx, configID, userID)
}

// EnableBackupConfig enables a backup configuration
func (s *MainService) EnableBackupConfig(ctx context.Context, configID string, userID string) error {
	return s.configService.Enable(ctx, configID, userID)
}

// DisableBackupConfig disables a backup configuration
func (s *MainService) DisableBackupConfig(ctx context.Context, configID string, userID string) error {
	return s.configService.Disable(ctx, configID, userID)
}

// ExecuteBackupConfig executes a backup configuration manually
func (s *MainService) ExecuteBackupConfig(ctx context.Context, configID string, userID string) (*BackupJob, error) {
	return s.configService.Execute(ctx, configID, userID)
}

// =============================================================================
// Backup Job API
// =============================================================================

// CreateBackupJob creates a new backup job
func (s *MainService) CreateBackupJob(ctx context.Context, req CreateBackupJobRequest, userID string) (*BackupJob, error) {
	return s.jobService.Create(ctx, req, userID)
}

// GetBackupJob retrieves a backup job by ID
func (s *MainService) GetBackupJob(ctx context.Context, jobID string) (*BackupJob, error) {
	return s.jobService.Get(ctx, jobID)
}

// ListBackupJobs lists backup jobs with optional filtering
func (s *MainService) ListBackupJobs(ctx context.Context, filters JobFilter) ([]*BackupJob, int, error) {
	return s.jobService.List(ctx, filters)
}

// ExecuteBackupJob executes a backup job
func (s *MainService) ExecuteBackupJob(ctx context.Context, jobID string, userID string) error {
	return s.jobService.Execute(ctx, jobID, userID)
}

// CancelBackupJob cancels a running backup job
func (s *MainService) CancelBackupJob(ctx context.Context, jobID string, userID string) error {
	return s.jobService.Cancel(ctx, jobID, userID)
}

// RetryBackupJob retries a failed backup job
func (s *MainService) RetryBackupJob(ctx context.Context, jobID string, userID string) (*BackupJob, error) {
	return s.jobService.Retry(ctx, jobID, userID)
}

// DeleteBackupJob deletes a backup job
func (s *MainService) DeleteBackupJob(ctx context.Context, jobID string, userID string) error {
	return s.jobService.Delete(ctx, jobID, userID)
}

// =============================================================================
// Backup Artifact API
// =============================================================================

// CreateBackupArtifact creates a new backup artifact
func (s *MainService) CreateBackupArtifact(ctx context.Context, req CreateArtifactRequest, userID string) (*BackupArtifact, error) {
	return s.artifactService.Create(ctx, req, userID)
}

// GetBackupArtifact retrieves a backup artifact by ID
func (s *MainService) GetBackupArtifact(ctx context.Context, artifactID string) (*BackupArtifact, error) {
	return s.artifactService.Get(ctx, artifactID)
}

// ListBackupArtifacts lists backup artifacts with optional filtering
func (s *MainService) ListBackupArtifacts(ctx context.Context, filters ArtifactFilter) ([]*BackupArtifact, int, error) {
	return s.artifactService.List(ctx, filters)
}

// DeleteBackupArtifact deletes a backup artifact
func (s *MainService) DeleteBackupArtifact(ctx context.Context, artifactID string, userID string) error {
	return s.artifactService.Delete(ctx, artifactID, userID)
}

// LockBackupArtifact locks a backup artifact
func (s *MainService) LockBackupArtifact(ctx context.Context, artifactID string, userID string, reason string) error {
	return s.artifactService.Lock(ctx, artifactID, userID, reason)
}

// UnlockBackupArtifact unlocks a backup artifact
func (s *MainService) UnlockBackupArtifact(ctx context.Context, artifactID string, userID string) error {
	return s.artifactService.Unlock(ctx, artifactID, userID)
}

// VerifyBackupArtifact verifies a backup artifact's integrity
func (s *MainService) VerifyBackupArtifact(ctx context.Context, artifactID string) error {
	return s.artifactService.Verify(ctx, artifactID)
}

// DownloadBackupArtifact downloads a backup artifact
func (s *MainService) DownloadBackupArtifact(ctx context.Context, artifactID string) (io.Reader, error) {
	return s.artifactService.Download(ctx, artifactID)
}

// GetBackupArtifactManifest retrieves the manifest for a backup artifact
func (s *MainService) GetBackupArtifactManifest(ctx context.Context, artifactID string) (*BackupManifest, error) {
	return s.artifactService.GetManifest(ctx, artifactID)
}

// CleanupExpiredArtifacts cleans up expired backup artifacts
func (s *MainService) CleanupExpiredArtifacts(ctx context.Context, retentionDays int) (int, error) {
	return s.artifactService.CleanupExpired(ctx, retentionDays)
}

// =============================================================================
// Restore API
// =============================================================================

// CreateRestore creates a new restore operation
func (s *MainService) CreateRestore(ctx context.Context, req CreateRestoreRequest, userID string) (*BackupRestore, error) {
	return s.restoreService.Create(ctx, req, userID)
}

// GetRestore retrieves a restore operation by ID
func (s *MainService) GetRestore(ctx context.Context, restoreID string) (*BackupRestore, error) {
	return s.restoreService.Get(ctx, restoreID)
}

// ListRestores lists restore operations with optional filtering
func (s *MainService) ListRestores(ctx context.Context, filters RestoreFilter) ([]*BackupRestore, int, error) {
	return s.restoreService.List(ctx, filters)
}

// ExecuteRestore executes a restore operation
func (s *MainService) ExecuteRestore(ctx context.Context, restoreID string, userID string) error {
	return s.restoreService.Execute(ctx, restoreID, userID)
}

// CancelRestore cancels a running restore operation
func (s *MainService) CancelRestore(ctx context.Context, restoreID string, userID string) error {
	return s.restoreService.Cancel(ctx, restoreID, userID)
}

// RetryRestore retries a failed restore operation
func (s *MainService) RetryRestore(ctx context.Context, restoreID string, userID string) (*BackupRestore, error) {
	return s.restoreService.Retry(ctx, restoreID, userID)
}

// DeleteRestore deletes a restore operation
func (s *MainService) DeleteRestore(ctx context.Context, restoreID string, userID string) error {
	return s.restoreService.Delete(ctx, restoreID, userID)
}

// TestRestore performs a test restore to verify backup integrity
func (s *MainService) TestRestore(ctx context.Context, artifactID string) (*BackupRestore, error) {
	return s.restoreService.TestRestore(ctx, artifactID)
}

// VerifyRestore verifies a completed restore operation
func (s *MainService) VerifyRestore(ctx context.Context, restoreID string) (bool, error) {
	return s.restoreService.VerifyRestore(ctx, restoreID)
}

// Rollback performs a rollback to a previous state
func (s *MainService) Rollback(ctx context.Context, restoreID string, userID string) (*BackupRestore, error) {
	return s.restoreService.Rollback(ctx, restoreID, userID)
}

// =============================================================================
// Scheduling API
// =============================================================================

// ScheduleBackup schedules a backup configuration
func (s *MainService) ScheduleBackup(ctx context.Context, configID string, cronExpr string, userID string) error {
	// Get the configuration
	config, err := s.configService.Get(ctx, configID)
	if err != nil {
		return fmt.Errorf("failed to get backup configuration: %w", err)
	}

	// Update configuration
	config.IsScheduled = true
	config.CronExpression = &cronExpr

	// Calculate next run time
	nextRun, err := s.calculateNextCronRun(cronExpr)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}
	config.NextRunAt = &nextRun

	// Update in database
	// TODO: Implement update

	// Schedule the job
	if s.scheduler != nil {
		err = s.scheduler.ScheduleBackupJob(ctx, configID, cronExpr)
		if err != nil {
			return fmt.Errorf("failed to schedule backup job: %w", err)
		}
	}

	s.logger.Infof("Scheduled backup configuration: %s (cron: %s, next run: %s)", config.Name, cronExpr, nextRun.Format(time.RFC3339))

	return nil
}

// UnscheduleBackup unschedules a backup configuration
func (s *MainService) UnscheduleBackup(ctx context.Context, configID string, userID string) error {
	// Get the configuration
	config, err := s.configService.Get(ctx, configID)
	if err != nil {
		return fmt.Errorf("failed to get backup configuration: %w", err)
	}

	// Update configuration
	config.IsScheduled = false
	config.CronExpression = nil
	config.NextRunAt = nil

	// Update in database
	// TODO: Implement update

	// Unschedules the job
	if s.scheduler != nil {
		err = s.scheduler.UnscheduleBackupJob(ctx, configID)
		if err != nil {
			return fmt.Errorf("failed to unschedule backup job: %w", err)
		}
	}

	s.logger.Infof("Unscheduled backup configuration: %s", config.Name)

	return nil
}

// GetBackupSchedule gets the schedule for a backup configuration
func (s *MainService) GetBackupSchedule(ctx context.Context, configID string) (*BackupSchedule, error) {
	config, err := s.configService.Get(ctx, configID)
	if err != nil {
		return nil, fmt.Errorf("failed to get backup configuration: %w", err)
	}

	if !config.IsScheduled {
		return nil, fmt.Errorf("backup configuration is not scheduled")
	}

	return &BackupSchedule{
		ConfigurationID: config.ID,
		CronExpression:  config.CronExpression,
		NextRunAt:       config.NextRunAt,
		LastRunAt:       config.LastRunAt,
		Enabled:         config.Enabled,
	}, nil
}

// ListBackupSchedules lists all scheduled backups
func (s *MainService) ListBackupSchedules(ctx context.Context) ([]*BackupSchedule, error) {
	// Get all scheduled configurations
	filters := BackupConfigFilter{
		Scheduled: boolPtr(true),
	}

	configs, _, err := s.configService.List(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list backup configurations: %w", err)
	}

	schedules := make([]*BackupSchedule, len(configs))
	for i, config := range configs {
		schedules[i] = &BackupSchedule{
			ConfigurationID: config.ID,
			Name:            config.Name,
			CronExpression:  config.CronExpression,
			NextRunAt:       config.NextRunAt,
			LastRunAt:       config.LastRunAt,
			Enabled:         config.Enabled,
			BackupType:      config.BackupType,
		}
	}

	return schedules, nil
}

// =============================================================================
// Storage Provider API
// =============================================================================

// RegisterStorageProvider registers a new storage provider
func (s *MainService) RegisterStorageProvider(ctx context.Context, req RegisterStorageProviderRequest, userID string) error {
	// Validate the request
	if req.Name == "" {
		return fmt.Errorf("storage provider name is required")
	}

	if req.ProviderType != "local" && req.ProviderType != "s3" &&
		req.ProviderType != "minio" && req.ProviderType != "azure" && req.ProviderType != "gcs" {
		return fmt.Errorf("invalid provider type: %s", req.ProviderType)
	}

	// Create the storage adapter based on provider type
	adapter, err := s.createStorageAdapter(req)
	if err != nil {
		return fmt.Errorf("failed to create storage adapter: %w", err)
	}

	// Register the adapter
	s.RegisterStorageAdapter(req.Name, adapter)

	// TODO: Store the provider configuration in the database

	s.logger.Infof("Registered storage provider: %s (type: %s)", req.Name, req.ProviderType)

	return nil
}

// GetStorageProvider gets a storage provider by name
func (s *MainService) GetStorageProvider(ctx context.Context, providerName string) (*StorageProvider, error) {
	// TODO: Implement database retrieval
	// For now, return a placeholder
	return nil, fmt.Errorf("not implemented: Get storage provider")
}

// ListStorageProviders lists all registered storage providers
func (s *MainService) ListStorageProviders(ctx context.Context) ([]*StorageProvider, error) {
	// TODO: Implement database listing
	// For now, return the registered adapters
	providers := make([]*StorageProvider, 0, len(s.storageAdapters))
	for name, adapter := range s.storageAdapters {
		providers = append(providers, &StorageProvider{
			Name:      name,
			Type:      adapter.Name(),
			Enabled:   true,
			IsDefault: adapter == s.defaultAdapter,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
	}
	return providers, nil
}

// TestStorageProvider tests a storage provider connection
func (s *MainService) TestStorageProvider(ctx context.Context, providerName string) error {
	adapter, ok := s.storageAdapters[providerName]
	if !ok {
		return fmt.Errorf("storage provider %s not found", providerName)
	}

	// Test the adapter by trying to list files
	_, err := adapter.List(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to test storage provider: %w", err)
	}

	s.logger.Infof("Storage provider test passed: %s", providerName)
	return nil
}

// SetDefaultStorageProvider sets the default storage provider
func (s *MainService) SetDefaultStorageProvider(ctx context.Context, providerName string, userID string) error {
	adapter, ok := s.storageAdapters[providerName]
	if !ok {
		return fmt.Errorf("storage provider %s not found", providerName)
	}

	s.defaultAdapter = adapter
	s.artifactService.SetDefaultAdapter(adapter)

	// TODO: Update in database

	s.logger.Infof("Set default storage provider: %s", providerName)
	return nil
}

// =============================================================================
// Retention Policy API
// =============================================================================

// CreateRetentionPolicy creates a new retention policy
func (s *MainService) CreateRetentionPolicy(ctx context.Context, req CreateRetentionPolicyRequest, userID string) (*RetentionPolicy, error) {
	// Validate the request
	if req.Name == "" {
		return nil, fmt.Errorf("retention policy name is required")
	}

	if req.Scope != "global" && req.Scope != "server" &&
		req.Scope != "app" && req.Scope != "database" && req.Scope != "volume" {
		return nil, fmt.Errorf("invalid scope: %s", req.Scope)
	}

	// Validate that exactly one target is specified for non-global scope
	if req.Scope != "global" {
		targetCount := 0
		if req.ServerID != nil && *req.ServerID != "" {
			targetCount++
		}
		if req.AppID != nil && *req.AppID != "" {
			targetCount++
		}
		if req.DatabaseID != nil && *req.DatabaseID != "" {
			targetCount++
		}
		if req.VolumeID != nil && *req.VolumeID != "" {
			targetCount++
		}

		if targetCount != 1 {
			return nil, fmt.Errorf("exactly one target must be specified for non-global scope")
		}
	}

	// Set defaults
	if req.Priority == 0 {
		req.Priority = 100
	}
	if !req.Enabled {
		req.Enabled = true
	}

	// Generate ID
	policyID := uuid.NewString()

	now := time.Now()
	policy := &RetentionPolicy{
		ID:              policyID,
		Name:            req.Name,
		Description:     req.Description,
		Scope:           req.Scope,
		ServerID:        req.ServerID,
		AppID:           req.AppID,
		DatabaseID:      req.DatabaseID,
		VolumeID:        req.VolumeID,
		MaxBackups:      req.MaxBackups,
		RetentionDays:   req.RetentionDays,
		RetentionWeeks:  req.RetentionWeeks,
		RetentionMonths: req.RetentionMonths,
		CleanupSchedule: req.CleanupSchedule,
		Priority:        req.Priority,
		Enabled:         req.Enabled,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// TODO: Store in database

	s.logger.Infof("Created retention policy: %s (scope: %s)", policy.Name, policy.Scope)

	return policy, nil
}

// GetRetentionPolicy retrieves a retention policy by ID
func (s *MainService) GetRetentionPolicy(ctx context.Context, policyID string) (*RetentionPolicy, error) {
	// TODO: Implement database retrieval
	return nil, fmt.Errorf("not implemented: Get retention policy")
}

// ListRetentionPolicies lists retention policies with optional filtering
func (s *MainService) ListRetentionPolicies(ctx context.Context, filters RetentionPolicyFilter) ([]*RetentionPolicy, int, error) {
	// TODO: Implement database listing
	return []*RetentionPolicy{}, 0, nil
}

// UpdateRetentionPolicy updates a retention policy
func (s *MainService) UpdateRetentionPolicy(ctx context.Context, policyID string, req UpdateRetentionPolicyRequest, userID string) (*RetentionPolicy, error) {
	// TODO: Implement database update
	return nil, fmt.Errorf("not implemented: Update retention policy")
}

// DeleteRetentionPolicy deletes a retention policy
func (s *MainService) DeleteRetentionPolicy(ctx context.Context, policyID string, userID string) error {
	// TODO: Implement database deletion
	return fmt.Errorf("not implemented: Delete retention policy")
}

// EnableRetentionPolicy enables a retention policy
func (s *MainService) EnableRetentionPolicy(ctx context.Context, policyID string, userID string) error {
	// TODO: Implement enable
	return fmt.Errorf("not implemented: Enable retention policy")
}

// DisableRetentionPolicy disables a retention policy
func (s *MainService) DisableRetentionPolicy(ctx context.Context, policyID string, userID string) error {
	// TODO: Implement disable
	return fmt.Errorf("not implemented: Disable retention policy")
}

// ApplyRetentionPolicy applies a retention policy to cleanup old backups
func (s *MainService) ApplyRetentionPolicy(ctx context.Context, policyID string, userID string) (int, error) {
	policy, err := s.GetRetentionPolicy(ctx, policyID)
	if err != nil {
		return 0, fmt.Errorf("failed to get retention policy: %w", err)
	}

	return s.artifactService.ApplyRetentionPolicy(ctx, *policy)
}

// =============================================================================
// Utility Methods
// =============================================================================

// GetBackupSystemStatus gets the overall status of the backup system
func (s *MainService) GetBackupSystemStatus(ctx context.Context) (*BackupSystemStatus, error) {
	// Get counts for different entities
	configs, configCount, err := s.configService.List(ctx, BackupConfigFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to count backup configurations: %w", err)
	}

	jobs, jobCount, err := s.jobService.List(ctx, JobFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to count backup jobs: %w", err)
	}

	artifacts, artifactCount, err := s.artifactService.List(ctx, ArtifactFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to count backup artifacts: %w", err)
	}

	restores, restoreCount, err := s.restoreService.List(ctx, RestoreFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to count restore operations: %w", err)
	}

	// Count scheduled configurations
	scheduledCount := 0
	for _, config := range configs {
		if config.IsScheduled {
			scheduledCount++
		}
	}

	// Count running jobs
	runningJobs := 0
	for _, job := range jobs {
		if job.Status == BackupRunning {
			runningJobs++
		}
	}

	// Count verified artifacts
	verifiedArtifacts := 0
	for _, artifact := range artifacts {
		if artifact.IsVerified {
			verifiedArtifacts++
		}
	}

	// Count completed restores
	completedRestores := 0
	for _, restore := range restores {
		if restore.Status == "completed" {
			completedRestores++
		}
	}

	return &BackupSystemStatus{
		BackupConfigurations: BackupSystemStatusCounts{
			Total:     configCount,
			Scheduled: scheduledCount,
			Enabled:   0, // TODO: Count enabled
		},
		BackupJobs: BackupSystemStatusCounts{
			Total:   jobCount,
			Running: runningJobs,
			Pending: 0, // TODO: Count pending
			Failed:  0, // TODO: Count failed
		},
		BackupArtifacts: BackupSystemStatusCounts{
			Total:    artifactCount,
			Verified: verifiedArtifacts,
			Locked:   0, // TODO: Count locked
			Expired:  0, // TODO: Count expired
		},
		BackupRestores: BackupSystemStatusCounts{
			Total:     restoreCount,
			Completed: completedRestores,
			Failed:    0, // TODO: Count failed
		},
		StorageProviders: len(s.storageAdapters),
		LastCleanupAt:    nil, // TODO: Get last cleanup time
	}, nil
}

// GetBackupSystemStatistics gets detailed statistics for the backup system
func (s *MainService) GetBackupSystemStatistics(ctx context.Context, days int) (*BackupSystemStatistics, error) {
	// TODO: Implement detailed statistics collection
	return &BackupSystemStatistics{
		TimeRange:    fmt.Sprintf("last %d days", days),
		TotalBackups: 0,
		TotalSize:    0,
		SuccessRate:  0,
		AverageSize:  0,
		ByType:       make(map[string]int),
		ByStatus:     make(map[string]int),
		ByStorage:    make(map[string]int),
	}, nil
}

// =============================================================================
// Helper Methods
// =============================================================================

// createStorageAdapter creates a storage adapter from a request
func (s *MainService) createStorageAdapter(req RegisterStorageProviderRequest) (StorageAdapter, error) {
	var config *StorageConfig
	if len(req.Config) > 0 {
		var storageConfig StorageConfig
		err := json.Unmarshal(req.Config, &storageConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to parse storage config: %w", err)
		}
		config = &storageConfig
	}

	return CreateStorageAdapter(req.ProviderType, config)
}

// calculateNextCronRun calculates the next run time for a cron expression
func (s *MainService) calculateNextCronRun(cronExpr string) (time.Time, error) {
	// TODO: Implement cron parsing
	// For now, return a time 1 hour from now as a placeholder
	return time.Now().Add(1 * time.Hour), nil
}

// CreateStorageAdapter creates a storage adapter based on provider type and config
func CreateStorageAdapter(providerType string, config *StorageConfig) (StorageAdapter, error) {
	switch providerType {
	case "local":
		if config == nil || config.Local == nil {
			return nil, fmt.Errorf("local storage config is required")
		}
		return NewLocalStorageBackend(config.Local.BasePath)
	case "s3":
		if config == nil || config.S3 == nil {
			return nil, fmt.Errorf("S3 storage config is required")
		}
		// Note: This uses the existing S3Adapter from the package
		// We need to adapt it to our StorageAdapter interface
		return NewS3StorageAdapter(config.S3)
	case "minio":
		if config == nil || config.MinIO == nil {
			return nil, fmt.Errorf("MinIO storage config is required")
		}
		return NewMinIOStorageAdapter(config.MinIO)
	case "azure":
		if config == nil || config.Azure == nil {
			return nil, fmt.Errorf("Azure storage config is required")
		}
		return NewAzureStorageAdapter(config.Azure)
	case "gcs":
		if config == nil || config.GCS == nil {
			return nil, fmt.Errorf("GCS storage config is required")
		}
		return NewGCSStorageAdapter(config.GCS)
	default:
		return nil, fmt.Errorf("unsupported storage provider type: %s", providerType)
	}
}

// =============================================================================
// Data Types
// =============================================================================

// BackupSchedule represents a backup schedule
type BackupSchedule struct {
	ConfigurationID string     `json:"configurationId"`
	Name            string     `json:"name,omitempty"`
	CronExpression  *string    `json:"cronExpression,omitempty"`
	NextRunAt       *time.Time `json:"nextRunAt,omitempty"`
	LastRunAt       *time.Time `json:"lastRunAt,omitempty"`
	Enabled         bool       `json:"enabled"`
	BackupType      BackupType `json:"backupType"`
}

// StorageProvider represents a storage provider configuration
type StorageProvider struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	Config         json.RawMessage `json:"config,omitempty"`
	Enabled        bool            `json:"enabled"`
	IsDefault      bool            `json:"isDefault"`
	LastTestAt     *time.Time      `json:"lastTestAt,omitempty"`
	LastTestStatus *string         `json:"lastTestStatus,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// RegisterStorageProviderRequest represents a request to register a storage provider
type RegisterStorageProviderRequest struct {
	Name         string          `json:"name"`
	ProviderType string          `json:"providerType"`
	Config       json.RawMessage `json:"config,omitempty"`
	Enabled      bool            `json:"enabled,omitempty"`
	IsDefault    bool            `json:"isDefault,omitempty"`
}

// RetentionPolicy represents a retention policy
type RetentionPolicy struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Description     string     `json:"description,omitempty"`
	Scope           string     `json:"scope"`
	ServerID        *string    `json:"serverId,omitempty"`
	AppID           *string    `json:"appId,omitempty"`
	DatabaseID      *string    `json:"databaseId,omitempty"`
	VolumeID        *string    `json:"volumeId,omitempty"`
	MaxBackups      int        `json:"maxBackups,omitempty"`
	RetentionDays   int        `json:"retentionDays,omitempty"`
	RetentionWeeks  int        `json:"retentionWeeks,omitempty"`
	RetentionMonths int        `json:"retentionMonths,omitempty"`
	CleanupSchedule *string    `json:"cleanupSchedule,omitempty"`
	LastCleanupAt   *time.Time `json:"lastCleanupAt,omitempty"`
	NextCleanupAt   *time.Time `json:"nextCleanupAt,omitempty"`
	Priority        int        `json:"priority"`
	Enabled         bool       `json:"enabled"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// CreateRetentionPolicyRequest represents a request to create a retention policy
type CreateRetentionPolicyRequest struct {
	Name            string  `json:"name"`
	Description     string  `json:"description,omitempty"`
	Scope           string  `json:"scope"`
	ServerID        *string `json:"serverId,omitempty"`
	AppID           *string `json:"appId,omitempty"`
	DatabaseID      *string `json:"databaseId,omitempty"`
	VolumeID        *string `json:"volumeId,omitempty"`
	MaxBackups      int     `json:"maxBackups,omitempty"`
	RetentionDays   int     `json:"retentionDays,omitempty"`
	RetentionWeeks  int     `json:"retentionWeeks,omitempty"`
	RetentionMonths int     `json:"retentionMonths,omitempty"`
	CleanupSchedule *string `json:"cleanupSchedule,omitempty"`
	Priority        int     `json:"priority,omitempty"`
	Enabled         bool    `json:"enabled,omitempty"`
}

// UpdateRetentionPolicyRequest represents a request to update a retention policy
type UpdateRetentionPolicyRequest struct {
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	MaxBackups      *int    `json:"maxBackups,omitempty"`
	RetentionDays   *int    `json:"retentionDays,omitempty"`
	RetentionWeeks  *int    `json:"retentionWeeks,omitempty"`
	RetentionMonths *int    `json:"retentionMonths,omitempty"`
	CleanupSchedule *string `json:"cleanupSchedule,omitempty"`
	Priority        *int    `json:"priority,omitempty"`
	Enabled         *bool   `json:"enabled,omitempty"`
}

// RetentionPolicyFilter represents filters for listing retention policies
type RetentionPolicyFilter struct {
	Scope      *string
	ServerID   *string
	AppID      *string
	DatabaseID *string
	VolumeID   *string
	Enabled    *bool
	Search     *string
	Page       int
	PerPage    int
}

// BackupSystemStatus represents the overall status of the backup system
type BackupSystemStatus struct {
	BackupConfigurations BackupSystemStatusCounts `json:"backupConfigurations"`
	BackupJobs           BackupSystemStatusCounts `json:"backupJobs"`
	BackupArtifacts      BackupSystemStatusCounts `json:"backupArtifacts"`
	BackupRestores       BackupSystemStatusCounts `json:"backupRestores"`
	StorageProviders     int                      `json:"storageProviders"`
	LastCleanupAt        *time.Time               `json:"lastCleanupAt,omitempty"`
}

// BackupSystemStatusCounts represents counts for a backup system entity
type BackupSystemStatusCounts struct {
	Total     int `json:"total"`
	Scheduled int `json:"scheduled,omitempty"`
	Enabled   int `json:"enabled,omitempty"`
	Running   int `json:"running,omitempty"`
	Pending   int `json:"pending,omitempty"`
	Failed    int `json:"failed,omitempty"`
	Verified  int `json:"verified,omitempty"`
	Locked    int `json:"locked,omitempty"`
	Expired   int `json:"expired,omitempty"`
	Completed int `json:"completed,omitempty"`
}

// BackupSystemStatistics represents detailed statistics for the backup system
type BackupSystemStatistics struct {
	TimeRange    string         `json:"timeRange"`
	TotalBackups int            `json:"totalBackups"`
	TotalSize    int64          `json:"totalSize"`
	SuccessRate  float64        `json:"successRate"`
	AverageSize  float64        `json:"averageSize"`
	ByType       map[string]int `json:"byType,omitempty"`
	ByStatus     map[string]int `json:"byStatus,omitempty"`
	ByStorage    map[string]int `json:"byStorage,omitempty"`
}

// Helper functions

func boolPtr(b bool) *bool {
	return &b
}
