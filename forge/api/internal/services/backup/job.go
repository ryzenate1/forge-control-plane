package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gamepanel/forge/internal/store"
)

// BackupJob represents a backup job execution
type BackupJob struct {
	ID                 string          `json:"id"`
	ConfigurationID    *string         `json:"configurationId,omitempty"`
	JobType            BackupType      `json:"jobType"`
	ServerID           *string         `json:"serverId,omitempty"`
	AppID              *string         `json:"appId,omitempty"`
	DatabaseID         *string         `json:"databaseId,omitempty"`
	VolumeID           *string         `json:"volumeId,omitempty"`
	Name               string          `json:"name"`
	Description        string          `json:"description,omitempty"`
	Status             BackupStatus    `json:"status"`
	StorageProvider    string          `json:"storageProvider,omitempty"`
	StorageConfig      json.RawMessage `json:"storageConfig,omitempty"`
	StartedAt          *time.Time      `json:"startedAt,omitempty"`
	CompletedAt        *time.Time      `json:"completedAt,omitempty"`
	DurationSeconds    *int            `json:"durationSeconds,omitempty"`
	BytesProcessed     int64           `json:"bytesProcessed"`
	TotalBytes         *int64          `json:"totalBytes,omitempty"`
	CurrentPhase       string          `json:"currentPhase,omitempty"`
	ProgressPercentage float64         `json:"progressPercentage"`
	ErrorMessage       *string         `json:"errorMessage,omitempty"`
	RetryCount         int             `json:"retryCount"`
	MaxRetries         int             `json:"maxRetries"`
	LastRetryAt        *time.Time      `json:"lastRetryAt,omitempty"`
	TriggeredBy        string          `json:"triggeredBy"`
	TriggeredByUserID  *string         `json:"triggeredByUserId,omitempty"`
	NodeID             *string         `json:"nodeId,omitempty"`
	BeaconTaskID       *string         `json:"beaconTaskId,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

// CreateBackupJobRequest represents a request to create a backup job
type CreateBackupJobRequest struct {
	ConfigurationID    *string         `json:"configurationId,omitempty"`
	JobType            BackupType      `json:"jobType"`
	ServerID           *string         `json:"serverId,omitempty"`
	AppID              *string         `json:"appId,omitempty"`
	DatabaseID         *string         `json:"databaseId,omitempty"`
	VolumeID           *string         `json:"volumeId,omitempty"`
	Name               string          `json:"name"`
	Description        string          `json:"description,omitempty"`
	StorageProvider    string          `json:"storageProvider,omitempty"`
	StorageConfig      json.RawMessage `json:"storageConfig,omitempty"`
	CompressionEnabled *bool           `json:"compressionEnabled,omitempty"`
	EncryptionEnabled  *bool           `json:"encryptionEnabled,omitempty"`
	EncryptionKeyID    *string         `json:"encryptionKeyId,omitempty"`
	MaxRetries         int             `json:"maxRetries,omitempty"`
	TriggeredBy        string          `json:"triggeredBy,omitempty"`
}

// JobFilter represents filters for listing backup jobs
type JobFilter struct {
	ConfigurationID *string
	JobType         *BackupType
	ServerID        *string
	AppID           *string
	DatabaseID      *string
	VolumeID        *string
	Status          *BackupStatus
	TriggeredBy     *string
	Search          *string
	StartDate       *time.Time
	EndDate         *time.Time
	Page            int
	PerPage         int
}

// JobService handles backup job management and execution
type JobService struct {
	store           *store.Store
	logger          Logger
	configService   *ConfigService
	artifactService *ArtifactService
	restoreService  *RestoreService
	beaconClient    BeaconClient
	scheduler       Scheduler
}

// NewJobService creates a new JobService
func NewJobService(store *store.Store, logger Logger) *JobService {
	return &JobService{
		store:  store,
		logger: logger,
		// Other services will be set separately to avoid circular dependencies
	}
}

// SetConfigService sets the config service
func (s *JobService) SetConfigService(configService *ConfigService) {
	s.configService = configService
}

// SetArtifactService sets the artifact service
func (s *JobService) SetArtifactService(artifactService *ArtifactService) {
	s.artifactService = artifactService
}

// SetRestoreService sets the restore service
func (s *JobService) SetRestoreService(restoreService *RestoreService) {
	s.restoreService = restoreService
}

// SetBeaconClient sets the beacon client
func (s *JobService) SetBeaconClient(beaconClient BeaconClient) {
	s.beaconClient = beaconClient
}

// SetScheduler sets the scheduler
func (s *JobService) SetScheduler(scheduler Scheduler) {
	s.scheduler = scheduler
}

// Create creates a new backup job
func (s *JobService) Create(ctx context.Context, req CreateBackupJobRequest, userID string) (*BackupJob, error) {
	// Validate the request
	if req.Name == "" {
		return nil, fmt.Errorf("backup job name is required")
	}

	// Validate job type
	if req.JobType != BackupTypeApp && req.JobType != BackupTypeVolume &&
		req.JobType != BackupTypeDatabase && req.JobType != BackupTypeServer {
		return nil, fmt.Errorf("invalid job type: %s", req.JobType)
	}

	// Validate that exactly one target is specified
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
		return nil, fmt.Errorf("exactly one target (server, app, database, or volume) must be specified")
	}

	// Set defaults
	if req.TriggeredBy == "" {
		req.TriggeredBy = "manual"
	}
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}

	// Generate ID
	jobID := uuid.NewString()

	now := time.Now()
	job := &BackupJob{
		ID:                 jobID,
		ConfigurationID:    req.ConfigurationID,
		JobType:            req.JobType,
		ServerID:           req.ServerID,
		AppID:              req.AppID,
		DatabaseID:         req.DatabaseID,
		VolumeID:           req.VolumeID,
		Name:               req.Name,
		Description:        req.Description,
		Status:             BackupPending,
		BytesProcessed:     0,
		ProgressPercentage: 0,
		RetryCount:         0,
		MaxRetries:         req.MaxRetries,
		TriggeredBy:        req.TriggeredBy,
		TriggeredByUserID:  &userID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	s.logger.Infof("Created backup job: %s (type: %s, target: %s)", job.Name, job.JobType, job.getTargetDescription())

	return job, nil
}

// CreateFromConfig creates a backup job from a configuration
func (s *JobService) CreateFromConfig(ctx context.Context, config *BackupConfig, userID string) (*BackupJob, error) {
	if config == nil {
		return nil, fmt.Errorf("backup configuration is required")
	}

	req := CreateBackupJobRequest{
		ConfigurationID:    &config.ID,
		JobType:            config.BackupType,
		ServerID:           config.ServerID,
		AppID:              config.AppID,
		DatabaseID:         config.DatabaseID,
		VolumeID:           config.VolumeID,
		Name:               fmt.Sprintf("%s-%s", config.Name, time.Now().Format("20060102-150405")),
		Description:        fmt.Sprintf("Backup from configuration: %s", config.Name),
		StorageProvider:    config.StorageProvider,
		StorageConfig:      config.StorageConfig,
		CompressionEnabled: &config.CompressionEnabled,
		EncryptionEnabled:  &config.EncryptionEnabled,
		EncryptionKeyID:    config.EncryptionKeyID,
		MaxRetries:         3,
		TriggeredBy:        "schedule",
	}

	return s.Create(ctx, req, userID)
}

// Get retrieves a backup job by ID
func (s *JobService) Get(ctx context.Context, jobID string) (*BackupJob, error) {
	// TODO: Implement database retrieval
	// Placeholder implementation
	return nil, fmt.Errorf("not implemented: Get backup job")
}

// List retrieves backup jobs with optional filtering
func (s *JobService) List(ctx context.Context, filters JobFilter) ([]*BackupJob, int, error) {
	// TODO: Implement database listing with filters
	// Placeholder implementation
	return []*BackupJob{}, 0, nil
}

// Update updates a backup job
func (s *JobService) Update(ctx context.Context, jobID string, updates map[string]interface{}) (*BackupJob, error) {
	// TODO: Implement database update
	// Placeholder implementation
	return nil, fmt.Errorf("not implemented: Update backup job")
}

// Execute executes a backup job
func (s *JobService) Execute(ctx context.Context, jobID string, userID string) error {
	job, err := s.Get(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get backup job: %w", err)
	}

	if job.Status != BackupPending && job.Status != BackupFailed {
		return fmt.Errorf("backup job is not in a state that can be executed (current: %s)", job.Status)
	}

	// Update job status to running
	job.Status = BackupRunning
	now := time.Now()
	job.StartedAt = &now
	job.CurrentPhase = "preparing"
	job.ProgressPercentage = 0
	job.BytesProcessed = 0

	// TODO: Update in database

	s.logger.Infof("Starting backup job execution: %s (type: %s)", job.Name, job.JobType)

	// Execute the appropriate backup based on job type
	switch job.JobType {
	case BackupTypeApp:
		err = s.executeAppBackup(ctx, job)
	case BackupTypeVolume:
		err = s.executeVolumeBackup(ctx, job)
	case BackupTypeDatabase:
		err = s.executeDatabaseBackup(ctx, job)
	case BackupTypeServer:
		err = s.executeServerBackup(ctx, job)
	default:
		err = fmt.Errorf("unsupported backup type: %s", job.JobType)
	}

	if err != nil {
		// Handle retry logic
		if job.RetryCount < job.MaxRetries {
			job.RetryCount++
			job.Status = BackupPending
			now = time.Now()
			job.LastRetryAt = &now
			errMsg := err.Error()
			job.ErrorMessage = &errMsg
			// TODO: Update in database
			s.logger.Warnf("Backup job %s failed, retry %d/%d: %v", job.Name, job.RetryCount, job.MaxRetries, err)
			return fmt.Errorf("backup failed, will retry: %w", err)
		}

		// Max retries exceeded
		job.Status = BackupFailed
		errMsg := err.Error()
		job.ErrorMessage = &errMsg
		now = time.Now()
		job.CompletedAt = &now
		// TODO: Update in database
		s.logger.Errorf("Backup job %s failed after %d retries: %v", job.Name, job.MaxRetries, err)
		return fmt.Errorf("backup failed after max retries: %w", err)
	}

	// Backup completed successfully
	job.Status = BackupCompleted
	now = time.Now()
	job.CompletedAt = &now
	job.ProgressPercentage = 100

	// Calculate duration
	if job.StartedAt != nil {
		duration := int(time.Since(*job.StartedAt).Seconds())
		job.DurationSeconds = &duration
	}

	// TODO: Update in database
	s.logger.Infof("Backup job %s completed successfully in %d seconds", job.Name, *job.DurationSeconds)

	return nil
}

// Cancel cancels a running backup job
func (s *JobService) Cancel(ctx context.Context, jobID string, userID string) error {
	job, err := s.Get(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get backup job: %w", err)
	}

	if job.Status != BackupRunning && job.Status != BackupPending {
		return fmt.Errorf("backup job is not in a cancellable state (current: %s)", job.Status)
	}

	// TODO: Implement cancellation logic
	// This would involve:
	// 1. Updating job status to cancelled
	// 2. Stopping any running beacon tasks
	// 3. Cleaning up temporary files

	job.Status = BackupFailed // Using failed for now, could add cancelled status
	job.ErrorMessage = stringPtr("Backup cancelled by user")
	now := time.Now()
	job.CompletedAt = &now

	s.logger.Infof("Backup job %s cancelled by user %s", job.Name, userID)

	return nil
}

// Retry retries a failed backup job
func (s *JobService) Retry(ctx context.Context, jobID string, userID string) (*BackupJob, error) {
	job, err := s.Get(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get backup job: %w", err)
	}

	if job.Status != BackupFailed && job.Status != BackupPending {
		return nil, fmt.Errorf("backup job is not in a retryable state (current: %s)", job.Status)
	}

	// Reset job state for retry
	job.Status = BackupPending
	job.RetryCount = 0
	job.LastRetryAt = nil
	job.ErrorMessage = nil
	job.StartedAt = nil
	job.CompletedAt = nil
	job.DurationSeconds = nil
	job.BytesProcessed = 0
	job.ProgressPercentage = 0
	job.CurrentPhase = ""

	// TODO: Update in database

	// Execute the job
	err = s.Execute(ctx, jobID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to execute retry: %w", err)
	}

	return job, nil
}

// Delete deletes a backup job
func (s *JobService) Delete(ctx context.Context, jobID string, userID string) error {
	// TODO: Implement deletion
	// This should also clean up any associated artifacts if they exist
	return fmt.Errorf("not implemented: Delete backup job")
}

// executeAppBackup executes an app backup
func (s *JobService) executeAppBackup(ctx context.Context, job *BackupJob) error {
	s.logger.Infof("Executing app backup for job: %s", job.Name)

	// Update phase
	job.CurrentPhase = "validating"
	// TODO: Update in database

	// Validate app exists
	if job.AppID == nil || *job.AppID == "" {
		return fmt.Errorf("app ID is required for app backup")
	}

	// Get app details from database
	// TODO: Implement app retrieval

	// Update phase
	job.CurrentPhase = "preparing storage"
	// TODO: Update in database

	// Prepare storage adapter
	storageAdapter, err := s.prepareStorageAdapter(job)
	if err != nil {
		return fmt.Errorf("failed to prepare storage adapter: %w", err)
	}

	// Update phase
	job.CurrentPhase = "creating backup"
	// TODO: Update in database

	// Execute backup via beacon
	if s.beaconClient != nil {
		// Determine which node to use
		nodeID, err := s.determineNodeForApp(*job.AppID)
		if err != nil {
			return fmt.Errorf("failed to determine node for app: %w", err)
		}

		job.NodeID = &nodeID
		// TODO: Update in database

		// Execute backup command on node via beacon
		taskID, err := s.beaconClient.ExecuteBackup(ctx, nodeID, BackupTypeApp, *job.AppID, job.Name, storageAdapter)
		if err != nil {
			return fmt.Errorf("failed to execute backup on node: %w", err)
		}

		job.BeaconTaskID = &taskID
		// TODO: Update in database

		// Wait for task completion
		err = s.waitForBeaconTaskCompletion(ctx, taskID)
		if err != nil {
			return fmt.Errorf("beacon task failed: %w", err)
		}

		// Get task result
		result, err := s.beaconClient.GetBackupResult(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get backup result: %w", err)
		}

		// Create artifact from result
		artifact, err := s.artifactService.CreateFromBackupResult(ctx, job, result)
		if err != nil {
			return fmt.Errorf("failed to create artifact: %w", err)
		}

		job.CurrentPhase = "verifying backup"
		// TODO: Update in database

		// Verify the backup
		err = s.artifactService.Verify(ctx, artifact.ID)
		if err != nil {
			return fmt.Errorf("failed to verify backup: %w", err)
		}

		job.CurrentPhase = "completing"
		// TODO: Update in database

	} else {
		return fmt.Errorf("beacon client not available")
	}

	return nil
}

// executeVolumeBackup executes a volume backup
func (s *JobService) executeVolumeBackup(ctx context.Context, job *BackupJob) error {
	s.logger.Infof("Executing volume backup for job: %s", job.Name)

	// Similar implementation to app backup but for volumes
	// Update phase
	job.CurrentPhase = "validating"

	if job.VolumeID == nil || *job.VolumeID == "" {
		return fmt.Errorf("volume ID is required for volume backup")
	}

	// Prepare storage adapter
	storageAdapter, err := s.prepareStorageAdapter(job)
	if err != nil {
		return fmt.Errorf("failed to prepare storage adapter: %w", err)
	}

	// Execute via beacon
	if s.beaconClient != nil {
		nodeID, err := s.determineNodeForVolume(*job.VolumeID)
		if err != nil {
			return fmt.Errorf("failed to determine node for volume: %w", err)
		}

		job.NodeID = &nodeID

		taskID, err := s.beaconClient.ExecuteBackup(ctx, nodeID, BackupTypeVolume, *job.VolumeID, job.Name, storageAdapter)
		if err != nil {
			return fmt.Errorf("failed to execute backup on node: %w", err)
		}

		job.BeaconTaskID = &taskID

		err = s.waitForBeaconTaskCompletion(ctx, taskID)
		if err != nil {
			return fmt.Errorf("beacon task failed: %w", err)
		}

		result, err := s.beaconClient.GetBackupResult(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get backup result: %w", err)
		}

		artifact, err := s.artifactService.CreateFromBackupResult(ctx, job, result)
		if err != nil {
			return fmt.Errorf("failed to create artifact: %w", err)
		}

		err = s.artifactService.Verify(ctx, artifact.ID)
		if err != nil {
			return fmt.Errorf("failed to verify backup: %w", err)
		}

	} else {
		return fmt.Errorf("beacon client not available")
	}

	return nil
}

// executeDatabaseBackup executes a database backup
func (s *JobService) executeDatabaseBackup(ctx context.Context, job *BackupJob) error {
	s.logger.Infof("Executing database backup for job: %s", job.Name)

	// Similar implementation but for databases
	job.CurrentPhase = "validating"

	if job.DatabaseID == nil || *job.DatabaseID == "" {
		return fmt.Errorf("database ID is required for database backup")
	}

	// Get database details to determine engine
	// TODO: Implement database retrieval
	databaseEngine := DatabasePostgres // Default, should be retrieved from DB

	// Prepare storage adapter
	storageAdapter, err := s.prepareStorageAdapter(job)
	if err != nil {
		return fmt.Errorf("failed to prepare storage adapter: %w", err)
	}

	// Execute via beacon
	if s.beaconClient != nil {
		nodeID, err := s.determineNodeForDatabase(*job.DatabaseID)
		if err != nil {
			return fmt.Errorf("failed to determine node for database: %w", err)
		}

		job.NodeID = &nodeID

		// For database backups, we need to pass the engine type
		taskID, err := s.beaconClient.ExecuteDatabaseBackup(ctx, nodeID, databaseEngine, *job.DatabaseID, job.Name, storageAdapter)
		if err != nil {
			return fmt.Errorf("failed to execute database backup on node: %w", err)
		}

		job.BeaconTaskID = &taskID

		err = s.waitForBeaconTaskCompletion(ctx, taskID)
		if err != nil {
			return fmt.Errorf("beacon task failed: %w", err)
		}

		result, err := s.beaconClient.GetBackupResult(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get backup result: %w", err)
		}

		artifact, err := s.artifactService.CreateFromBackupResult(ctx, job, result)
		if err != nil {
			return fmt.Errorf("failed to create artifact: %w", err)
		}

		// For database backups, we might want to test the restore
		if s.shouldTestDatabaseRestore(databaseEngine) {
			job.CurrentPhase = "testing restore"
			// TODO: Update in database

			_, err = s.restoreService.TestRestore(ctx, artifact.ID)
			if err != nil {
				s.logger.Warnf("Database restore test failed for artifact %s: %v", artifact.ID, err)
				// Don't fail the backup, just log the warning
			}
		}

	} else {
		return fmt.Errorf("beacon client not available")
	}

	return nil
}

// executeServerBackup executes a server backup
func (s *JobService) executeServerBackup(ctx context.Context, job *BackupJob) error {
	s.logger.Infof("Executing server backup for job: %s", job.Name)

	// Similar implementation but for entire servers
	job.CurrentPhase = "validating"

	if job.ServerID == nil || *job.ServerID == "" {
		return fmt.Errorf("server ID is required for server backup")
	}

	// Prepare storage adapter
	storageAdapter, err := s.prepareStorageAdapter(job)
	if err != nil {
		return fmt.Errorf("failed to prepare storage adapter: %w", err)
	}

	// Execute via beacon
	if s.beaconClient != nil {
		nodeID, err := s.determineNodeForServer(*job.ServerID)
		if err != nil {
			return fmt.Errorf("failed to determine node for server: %w", err)
		}

		job.NodeID = &nodeID

		taskID, err := s.beaconClient.ExecuteBackup(ctx, nodeID, BackupTypeServer, *job.ServerID, job.Name, storageAdapter)
		if err != nil {
			return fmt.Errorf("failed to execute backup on node: %w", err)
		}

		job.BeaconTaskID = &taskID

		err = s.waitForBeaconTaskCompletion(ctx, taskID)
		if err != nil {
			return fmt.Errorf("beacon task failed: %w", err)
		}

		result, err := s.beaconClient.GetBackupResult(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get backup result: %w", err)
		}

		artifact, err := s.artifactService.CreateFromBackupResult(ctx, job, result)
		if err != nil {
			return fmt.Errorf("failed to create artifact: %w", err)
		}

		err = s.artifactService.Verify(ctx, artifact.ID)
		if err != nil {
			return fmt.Errorf("failed to verify backup: %w", err)
		}

	} else {
		return fmt.Errorf("beacon client not available")
	}

	return nil
}

// prepareStorageAdapter prepares the storage adapter for a job
func (s *JobService) prepareStorageAdapter(job *BackupJob) (StorageAdapter, error) {
	// If job has a configuration, use its storage settings
	if job.ConfigurationID != nil && *job.ConfigurationID != "" {
		config, err := s.configService.Get(context.Background(), *job.ConfigurationID)
		if err != nil {
			return nil, fmt.Errorf("failed to get backup configuration: %w", err)
		}

		storageConfig, err := config.GetStorageConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get storage config: %w", err)
		}

		return CreateStorageAdapter(config.StorageProvider, storageConfig)
	}

	// Use job's storage settings
	if job.StorageProvider == "" {
		job.StorageProvider = "local"
	}

	var storageConfig *StorageConfig
	if len(job.StorageConfig) > 0 {
		var config StorageConfig
		err := json.Unmarshal(job.StorageConfig, &config)
		if err != nil {
			return nil, fmt.Errorf("failed to parse job storage config: %w", err)
		}
		storageConfig = &config
	}

	return CreateStorageAdapter(job.StorageProvider, storageConfig)
}

// determineNodeForApp determines which node to use for an app backup
func (s *JobService) determineNodeForApp(appID string) (string, error) {
	// TODO: Implement logic to find which node the app is running on
	// This would query the database for app placement information
	return "", fmt.Errorf("not implemented: determineNodeForApp")
}

// determineNodeForVolume determines which node to use for a volume backup
func (s *JobService) determineNodeForVolume(volumeID string) (string, error) {
	// TODO: Implement logic to find which node the volume is on
	return "", fmt.Errorf("not implemented: determineNodeForVolume")
}

// determineNodeForDatabase determines which node to use for a database backup
func (s *JobService) determineNodeForDatabase(databaseID string) (string, error) {
	// TODO: Implement logic to find which node the database is on
	return "", fmt.Errorf("not implemented: determineNodeForDatabase")
}

// determineNodeForServer determines which node to use for a server backup
func (s *JobService) determineNodeForServer(serverID string) (string, error) {
	// TODO: Implement logic to find which node the server is on
	return "", fmt.Errorf("not implemented: determineNodeForServer")
}

// waitForBeaconTaskCompletion waits for a beacon task to complete
func (s *JobService) waitForBeaconTaskCompletion(ctx context.Context, taskID string) error {
	// TODO: Implement polling or callback-based waiting
	// For now, just return nil as if it completed
	return nil
}

// shouldTestDatabaseRestore determines if we should test database restore
func (s *JobService) shouldTestDatabaseRestore(engine DatabaseEngine) bool {
	// Test restore for critical databases
	return engine == DatabasePostgres || engine == DatabaseMySQL
}

// getTargetDescription returns a description of the job target
func (j *BackupJob) getTargetDescription() string {
	if j.ServerID != nil && *j.ServerID != "" {
		return fmt.Sprintf("server:%s", *j.ServerID)
	}
	if j.AppID != nil && *j.AppID != "" {
		return fmt.Sprintf("app:%s", *j.AppID)
	}
	if j.DatabaseID != nil && *j.DatabaseID != "" {
		return fmt.Sprintf("database:%s", *j.DatabaseID)
	}
	if j.VolumeID != nil && *j.VolumeID != "" {
		return fmt.Sprintf("volume:%s", *j.VolumeID)
	}
	return "unknown"
}

// stringPtr is a helper to create a string pointer
func stringPtr(s string) *string {
	return &s
}
