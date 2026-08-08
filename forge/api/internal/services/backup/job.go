package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"gamepanel/forge/internal/daemon"
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
	daemonClient    *daemon.Client
}

func (s *JobService) SetDaemonClient(client *daemon.Client) {
	s.daemonClient = client
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

	if err := validateBackupTargets(req.JobType, req.ServerID, req.AppID, req.DatabaseID, req.VolumeID); err != nil {
		return nil, err
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
	record, err := backupJobToStore(job)
	if err != nil {
		return nil, err
	}
	if err := s.store.CreateBackupJob(ctx, &record); err != nil {
		return nil, fmt.Errorf("persist backup job: %w", err)
	}
	job.CreatedAt = record.CreatedAt
	job.UpdatedAt = record.UpdatedAt

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
	record, err := s.store.GetBackupJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("get backup job: %w", err)
	}
	return backupJobFromStore(record), nil
}

// List retrieves backup jobs with optional filtering
func (s *JobService) List(ctx context.Context, filters JobFilter) ([]*BackupJob, int, error) {
	page, perPage := filters.Page, filters.PerPage
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}
	var jobType, status *string
	if filters.JobType != nil {
		value := string(*filters.JobType)
		jobType = &value
	}
	if filters.Status != nil {
		value := string(*filters.Status)
		status = &value
	}
	records, total, err := s.store.ListBackupJobs(ctx, store.BackupJobFilter{
		ConfigurationID: filters.ConfigurationID,
		JobType:         jobType,
		ServerID:        filters.ServerID,
		AppID:           filters.AppID,
		DatabaseID:      filters.DatabaseID,
		VolumeID:        filters.VolumeID,
		Status:          status,
		TriggeredBy:     filters.TriggeredBy,
		Search:          filters.Search,
		StartDate:       filters.StartDate,
		EndDate:         filters.EndDate,
		Limit:           perPage,
		Offset:          (page - 1) * perPage,
	})
	if err != nil {
		return nil, 0, err
	}
	jobs := make([]*BackupJob, 0, len(records))
	for _, record := range records {
		jobs = append(jobs, backupJobFromStore(record))
	}
	return jobs, total, nil
}

// Update updates a backup job using targeted column updates to avoid
// full-row overwrites clobbering concurrent writes.
func (s *JobService) Update(ctx context.Context, jobID string, updates map[string]interface{}) (*BackupJob, error) {
	job, err := s.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if value, ok := updates["status"].(string); ok {
		if err := s.store.UpdateBackupJobStatus(ctx, jobID, value); err != nil {
			return nil, err
		}
		job.Status = BackupStatus(value)
	}
	var phase string
	if value, ok := updates["currentPhase"].(string); ok {
		phase = value
		job.CurrentPhase = value
	}
	var progress float64
	if value, ok := updates["progressPercentage"].(float64); ok {
		if value < 0 || value > 100 {
			return nil, fmt.Errorf("progress percentage must be between 0 and 100")
		}
		progress = value
		job.ProgressPercentage = value
	}
	if value, ok := updates["bytesProcessed"].(int64); ok {
		job.BytesProcessed = value
	}
	if phase != "" || progress != 0 || updates["bytesProcessed"] != nil {
		if err := s.store.UpdateBackupJobProgress(ctx, jobID, phase, progress, job.BytesProcessed); err != nil {
			return nil, err
		}
	}
	if value, ok := updates["errorMessage"].(string); ok {
		job.ErrorMessage = &value
		if err := s.store.FailBackupJob(ctx, jobID, string(job.Status), job.RetryCount, job.LastRetryAt, job.CompletedAt, value); err != nil {
			return nil, err
		}
	}
	return job, nil
}

// Execute executes a backup job. The transition into the running state is an
// atomic compare-and-swap in the store, so concurrent executors (manual API
// calls and the scheduled worker) cannot run the same job twice.
func (s *JobService) Execute(ctx context.Context, jobID string, userID string) error {
	job, err := s.Get(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get backup job: %w", err)
	}

	if job.Status != BackupPending && job.Status != BackupFailed {
		return fmt.Errorf("backup job is not in a state that can be executed (current: %s)", job.Status)
	}

	claimed, err := s.store.ClaimBackupJobForExecution(ctx, jobID)
	if err != nil {
		return fmt.Errorf("claim backup job for execution: %w", err)
	}
	if !claimed {
		return fmt.Errorf("backup job %s is already running or was claimed by another worker", jobID)
	}

	// Re-read the job after the atomic claim so retry_count/error state is fresh.
	if job, err = s.Get(ctx, jobID); err != nil {
		return fmt.Errorf("reload backup job after claim: %w", err)
	}

	job.Status = BackupRunning
	job.CurrentPhase = "preparing"
	job.ProgressPercentage = 0
	job.BytesProcessed = 0
	if job.StartedAt == nil {
		now := time.Now()
		job.StartedAt = &now
	}

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
			now := time.Now()
			job.LastRetryAt = &now
			errMsg := err.Error()
			job.ErrorMessage = &errMsg
			if failErr := s.store.FailBackupJob(ctx, job.ID, string(BackupPending), job.RetryCount, job.LastRetryAt, nil, errMsg); failErr != nil {
				s.logger.Errorf("Failed to mark backup job %s for retry: %v", job.Name, failErr)
			}
			s.logger.Warnf("Backup job %s failed, retry %d/%d: %v", job.Name, job.RetryCount, job.MaxRetries, err)
			return fmt.Errorf("backup failed, will retry: %w", err)
		}

		// Max retries exceeded
		errMsg := err.Error()
		job.ErrorMessage = &errMsg
		now := time.Now()
		job.CompletedAt = &now
		job.Status = BackupFailed
		if failErr := s.store.FailBackupJob(ctx, job.ID, string(BackupFailed), job.RetryCount, job.LastRetryAt, job.CompletedAt, errMsg); failErr != nil {
			s.logger.Errorf("Failed to mark backup job %s failed: %v", job.Name, failErr)
		}
		s.logger.Errorf("Backup job %s failed after %d retries: %v", job.Name, job.MaxRetries, err)
		return fmt.Errorf("backup failed after max retries: %w", err)
	}

	// Backup completed successfully
	job.Status = BackupCompleted
	now := time.Now()
	job.CompletedAt = &now
	job.ProgressPercentage = 100

	// Calculate duration
	var duration *int
	if job.StartedAt != nil {
		d := int(time.Since(*job.StartedAt).Seconds())
		job.DurationSeconds = &d
		duration = job.DurationSeconds
	}

	if err := s.store.CompleteBackupJobSuccess(ctx, job.ID, duration, job.BytesProcessed); err != nil {
		return fmt.Errorf("complete backup job: %w", err)
	}
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

	job.Status = BackupCancelled
	job.ErrorMessage = stringPtr("Backup cancelled by user")
	now := time.Now()
	job.CompletedAt = &now
	if job.BeaconTaskID != nil && s.beaconClient != nil {
		if err := s.beaconClient.CancelTask(ctx, *job.BeaconTaskID); err != nil {
			return fmt.Errorf("cancel beacon task: %w", err)
		}
	}
	if err := s.store.FailBackupJob(ctx, job.ID, string(BackupCancelled), job.RetryCount, job.LastRetryAt, job.CompletedAt, "Backup cancelled by user"); err != nil {
		return err
	}

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

	if err := s.persistJob(ctx, job); err != nil {
		return nil, err
	}

	// Execute the job
	err = s.Execute(ctx, jobID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to execute retry: %w", err)
	}

	return job, nil
}

// Delete deletes a backup job
func (s *JobService) Delete(ctx context.Context, jobID string, userID string) error {
	if err := s.store.DeleteBackupJob(ctx, jobID); err != nil {
		return fmt.Errorf("delete backup job: %w", err)
	}
	s.logger.Infof("Deleted backup job %s by user %s", jobID, userID)
	return nil
}

// executeAppBackup executes an app backup
func (s *JobService) executeAppBackup(ctx context.Context, job *BackupJob) error {
	s.logger.Infof("Executing app backup for job: %s", job.Name)

	// Update phase
	job.CurrentPhase = "validating"
	if err := s.persistJob(ctx, job); err != nil {
		return err
	}

	// Validate app exists
	if job.AppID == nil || *job.AppID == "" {
		return fmt.Errorf("app ID is required for app backup")
	}
	if err := s.persistJob(ctx, job); err != nil {
		return err
	}

	app, err := s.store.GetApplication(ctx, *job.AppID)
	if err != nil {
		return fmt.Errorf("resolve app backup target: %w", err)
	}
	if app.ServerID == nil || *app.ServerID == "" {
		return fmt.Errorf("application %s has no backing server", *job.AppID)
	}
	if s.daemonClient != nil {
		target, err := s.store.ServerControlTarget(ctx, *app.ServerID)
		if err != nil {
			return fmt.Errorf("resolve app node: %w", err)
		}
		entry, err := s.daemonClient.CreateBackup(ctx, target.NodeURL, target.NodeToken, *app.ServerID, nil)
		if err != nil {
			return fmt.Errorf("beacon app backup: %w", err)
		}
		job.StorageProvider = "beacon"
		result := &BackupResult{
			ArtifactName: entry.Name, StoragePath: entry.Name, FileSize: entry.Size,
			FileHash: entry.Checksum, HashAlgorithm: "sha256", SourceType: "app",
			SourceID: *job.AppID, AppName: &app.Name, IsCompressed: true,
		}
		_, err = s.artifactService.CreateFromBackupResult(ctx, job, result)
		return err
	}

	// Update phase
	job.CurrentPhase = "preparing storage"
	if err := s.persistJob(ctx, job); err != nil {
		return err
	}

	// Prepare storage adapter
	storageAdapter, err := s.prepareStorageAdapter(job)
	if err != nil {
		return fmt.Errorf("failed to prepare storage adapter: %w", err)
	}

	// Update phase
	job.CurrentPhase = "creating backup"
	if err := s.persistJob(ctx, job); err != nil {
		return err
	}

	// Execute backup via beacon
	if s.beaconClient != nil {
		// Determine which node to use
		nodeID, err := s.determineNodeForApp(*job.AppID)
		if err != nil {
			return fmt.Errorf("failed to determine node for app: %w", err)
		}

		job.NodeID = &nodeID
		if err := s.persistJob(ctx, job); err != nil {
			return err
		}

		// Execute backup command on node via beacon
		taskID, err := s.beaconClient.ExecuteBackup(ctx, nodeID, BackupTypeApp, *job.AppID, job.Name, storageAdapter)
		if err != nil {
			return fmt.Errorf("failed to execute backup on node: %w", err)
		}

		job.BeaconTaskID = &taskID
		if err := s.persistJob(ctx, job); err != nil {
			return err
		}

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
		if err := s.persistJob(ctx, job); err != nil {
			return err
		}

		// Verify the backup
		err = s.artifactService.Verify(ctx, artifact.ID)
		if err != nil {
			return fmt.Errorf("failed to verify backup: %w", err)
		}

		job.CurrentPhase = "completing"
		if err := s.persistJob(ctx, job); err != nil {
			return err
		}

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
	if err := s.persistJob(ctx, job); err != nil {
		return err
	}

	if job.VolumeID == nil || *job.VolumeID == "" {
		return fmt.Errorf("volume ID is required for volume backup")
	}
	if job.ServerID == nil || *job.ServerID == "" {
		return fmt.Errorf("server ID is required for volume backup")
	}
	if s.daemonClient != nil {
		target, err := s.store.ServerControlTarget(ctx, *job.ServerID)
		if err != nil {
			return fmt.Errorf("resolve volume node: %w", err)
		}
		entry, err := s.daemonClient.BackupVolume(ctx, target.NodeURL, target.NodeToken, *job.ServerID, *job.VolumeID)
		if err != nil {
			return fmt.Errorf("beacon volume backup: %w", err)
		}
		job.StorageProvider = "beacon"
		result := &BackupResult{
			ArtifactName: entry.Name, StoragePath: entry.Name, FileSize: entry.Size,
			FileHash: entry.Checksum, HashAlgorithm: "sha256", SourceType: "volume",
			SourceID: *job.VolumeID, VolumeName: job.VolumeID, IsCompressed: true,
		}
		_, err = s.artifactService.CreateFromBackupResult(ctx, job, result)
		return err
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
			return fmt.Errorf("failed to determine node for volume: %w", err)
		}

		job.NodeID = &nodeID
		if err := s.persistJob(ctx, job); err != nil {
			return err
		}

		taskID, err := s.beaconClient.ExecuteBackup(ctx, nodeID, BackupTypeVolume, *job.VolumeID, job.Name, storageAdapter)
		if err != nil {
			return fmt.Errorf("failed to execute backup on node: %w", err)
		}

		job.BeaconTaskID = &taskID
		if err := s.persistJob(ctx, job); err != nil {
			return err
		}

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
	if err := s.persistJob(ctx, job); err != nil {
		return err
	}

	if job.DatabaseID == nil || *job.DatabaseID == "" {
		return fmt.Errorf("database ID is required for database backup")
	}
	if s.daemonClient != nil {
		databaseTarget, err := s.store.GetDBContainerBackupTarget(ctx, *job.DatabaseID)
		if err != nil {
			return fmt.Errorf("resolve database backup target: %w", err)
		}
		serverTarget, err := s.store.ServerControlTarget(ctx, databaseTarget.ServerID)
		if err != nil {
			return fmt.Errorf("resolve database node: %w", err)
		}
		entry, err := s.daemonClient.BackupDatabase(ctx, serverTarget.NodeURL, serverTarget.NodeToken,
			databaseTarget.ContainerID, databaseTarget.Engine, job.ID)
		if err != nil {
			return fmt.Errorf("beacon database backup: %w", err)
		}
		job.StorageProvider = "beacon"
		engine := DatabaseEngine(databaseTarget.Engine)
		result := &BackupResult{
			ArtifactName: entry.Name, StoragePath: entry.Name, FileSize: entry.Size,
			FileHash: entry.Checksum, HashAlgorithm: "sha256", SourceType: "database",
			SourceID: *job.DatabaseID, DatabaseEngine: &engine, IsCompressed: true,
		}
		if _, err := s.artifactService.CreateFromBackupResult(ctx, job, result); err != nil {
			return fmt.Errorf("persist database backup artifact: %w", err)
		}
		return nil
	}

	databaseTarget, err := s.store.GetDBContainerBackupTarget(ctx, *job.DatabaseID)
	if err != nil {
		return fmt.Errorf("resolve database backup target: %w", err)
	}
	databaseEngine := DatabaseEngine(databaseTarget.Engine)

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
		if err := s.persistJob(ctx, job); err != nil {
			return err
		}

		// For database backups, we need to pass the engine type
		taskID, err := s.beaconClient.ExecuteDatabaseBackup(ctx, nodeID, databaseEngine, *job.DatabaseID, job.Name, storageAdapter)
		if err != nil {
			return fmt.Errorf("failed to execute database backup on node: %w", err)
		}

		job.BeaconTaskID = &taskID
		if err := s.persistJob(ctx, job); err != nil {
			return err
		}

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
			if err := s.persistJob(ctx, job); err != nil {
				return err
			}

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
	if err := s.persistJob(ctx, job); err != nil {
		return err
	}

	if job.ServerID == nil || *job.ServerID == "" {
		return fmt.Errorf("server ID is required for server backup")
	}
	if s.daemonClient != nil {
		target, err := s.store.ServerControlTarget(ctx, *job.ServerID)
		if err != nil {
			return fmt.Errorf("resolve server backup target: %w", err)
		}
		entry, err := s.daemonClient.CreateBackup(ctx, target.NodeURL, target.NodeToken, *job.ServerID, nil)
		if err != nil {
			return fmt.Errorf("beacon server backup: %w", err)
		}
		job.StorageProvider = "beacon"
		result := &BackupResult{
			ArtifactName: entry.Name, StoragePath: entry.Name, FileSize: entry.Size,
			FileHash: entry.Checksum, HashAlgorithm: "sha256", SourceType: "server",
			SourceID: *job.ServerID, IsCompressed: true,
		}
		if _, err := s.artifactService.CreateFromBackupResult(ctx, job, result); err != nil {
			return fmt.Errorf("persist server backup artifact: %w", err)
		}
		return nil
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
	instances, err := s.store.ListInstancesByApp(context.Background(), appID)
	if err != nil {
		return "", err
	}
	for _, instance := range instances {
		if instance.NodeID != "" && instance.Status != "failed" && instance.Status != "stopped" {
			return instance.NodeID, nil
		}
	}
	return "", fmt.Errorf("application %s has no active instance", appID)
}

// determineNodeForDatabase determines which node to use for a database backup
func (s *JobService) determineNodeForDatabase(databaseID string) (string, error) {
	target, err := s.store.GetDBContainerBackupTarget(context.Background(), databaseID)
	if err != nil {
		return "", err
	}
	return s.determineNodeForServer(target.ServerID)
}

// determineNodeForServer determines which node to use for a server backup
func (s *JobService) determineNodeForServer(serverID string) (string, error) {
	return s.store.ServerNodeID(context.Background(), serverID)
}

// waitForBeaconTaskCompletion waits for a beacon task to complete
func (s *JobService) waitForBeaconTaskCompletion(ctx context.Context, taskID string) error {
	if s.beaconClient == nil {
		return fmt.Errorf("beacon client not available")
	}
	timeout := time.NewTimer(30 * time.Minute)
	defer timeout.Stop()
	backoff := time.Second
	for {
		status, err := s.beaconClient.GetTaskStatus(ctx, taskID)
		if err != nil {
			return fmt.Errorf("get beacon task status: %w", err)
		}
		switch strings.ToLower(status) {
		case "completed", "success":
			return nil
		case "failed", "error", "cancelled":
			return fmt.Errorf("beacon task %s ended with status %s", taskID, status)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for beacon task: %w", ctx.Err())
		case <-timeout.C:
			return fmt.Errorf("beacon task %s timed out", taskID)
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
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

func backupJobToStore(job *BackupJob) (store.BackupJob, error) {
	data, err := json.Marshal(job)
	if err != nil {
		return store.BackupJob{}, fmt.Errorf("encode backup job: %w", err)
	}
	return store.BackupJob{
		ID: job.ID, ConfigurationID: job.ConfigurationID, JobType: string(job.JobType),
		ServerID: job.ServerID, AppID: job.AppID, DatabaseID: job.DatabaseID,
		VolumeID: job.VolumeID, Name: job.Name, Description: job.Description,
		Status: string(job.Status), StartedAt: job.StartedAt, CompletedAt: job.CompletedAt,
		DurationSeconds: job.DurationSeconds, BytesProcessed: job.BytesProcessed,
		TotalBytes: job.TotalBytes, CurrentPhase: job.CurrentPhase,
		ErrorMessage: job.ErrorMessage, RetryCount: job.RetryCount,
		MaxRetries: job.MaxRetries, LastRetryAt: job.LastRetryAt,
		TriggeredBy: job.TriggeredBy, TriggeredByUserID: job.TriggeredByUserID,
		NodeID: job.NodeID, BeaconTaskID: job.BeaconTaskID,
		CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt, Data: data,
	}, nil
}

func backupJobFromStore(record store.BackupJob) *BackupJob {
	var job BackupJob
	_ = json.Unmarshal(record.Data, &job)
	job.ID = record.ID
	job.ConfigurationID = record.ConfigurationID
	job.JobType = BackupType(record.JobType)
	job.ServerID = record.ServerID
	job.AppID = record.AppID
	job.DatabaseID = record.DatabaseID
	job.VolumeID = record.VolumeID
	job.Name = record.Name
	job.Description = record.Description
	job.Status = BackupStatus(record.Status)
	job.StartedAt = record.StartedAt
	job.CompletedAt = record.CompletedAt
	job.DurationSeconds = record.DurationSeconds
	job.BytesProcessed = record.BytesProcessed
	job.TotalBytes = record.TotalBytes
	job.CurrentPhase = record.CurrentPhase
	job.ErrorMessage = record.ErrorMessage
	job.RetryCount = record.RetryCount
	job.MaxRetries = record.MaxRetries
	job.LastRetryAt = record.LastRetryAt
	job.TriggeredBy = record.TriggeredBy
	job.TriggeredByUserID = record.TriggeredByUserID
	job.NodeID = record.NodeID
	job.BeaconTaskID = record.BeaconTaskID
	job.CreatedAt = record.CreatedAt
	job.UpdatedAt = record.UpdatedAt
	return &job
}

func (s *JobService) persistJob(ctx context.Context, job *BackupJob) error {
	record, err := backupJobToStore(job)
	if err != nil {
		return err
	}
	if err := s.store.UpdateBackupJob(ctx, &record); err != nil {
		return fmt.Errorf("persist backup job: %w", err)
	}
	job.UpdatedAt = record.UpdatedAt
	return nil
}

// stringPtr is a helper to create a string pointer
func stringPtr(s string) *string {
	return &s
}
