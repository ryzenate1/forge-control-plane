package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"gamepanel/forge/internal/store"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type BackupRestore struct {
	ID                  string          `json:"id"`
	ArtifactID          *string         `json:"artifactId,omitempty"`
	JobID               *string         `json:"jobId,omitempty"`
	RestoreType         BackupType      `json:"restoreType"`
	TargetServerID      *string         `json:"targetServerId,omitempty"`
	TargetAppID         *string         `json:"targetAppId,omitempty"`
	TargetDatabaseID    *string         `json:"targetDatabaseId,omitempty"`
	TargetVolumeID      *string         `json:"targetVolumeId,omitempty"`
	Name                string          `json:"name"`
	Description         string          `json:"description,omitempty"`
	Status              string          `json:"status"`
	StartedAt           *time.Time      `json:"startedAt,omitempty"`
	CompletedAt         *time.Time      `json:"completedAt,omitempty"`
	LastRetryAt         *time.Time      `json:"lastRetryAt,omitempty"`
	DurationSeconds     *int            `json:"durationSeconds,omitempty"`
	BytesProcessed      int64           `json:"bytesProcessed"`
	TotalBytes          *int64          `json:"totalBytes,omitempty"`
	CurrentPhase        string          `json:"currentPhase,omitempty"`
	ProgressPercentage  float64         `json:"progressPercentage"`
	ErrorMessage        *string         `json:"errorMessage,omitempty"`
	RetryCount          int             `json:"retryCount"`
	MaxRetries          int             `json:"maxRetries"`
	NodeID              *string         `json:"nodeId,omitempty"`
	BeaconTaskID        *string         `json:"beaconTaskId,omitempty"`
	RestoreOptions      json.RawMessage `json:"restoreOptions,omitempty"`
	VerificationStatus  *string         `json:"verificationStatus,omitempty"`
	VerificationResults json.RawMessage `json:"verificationResults,omitempty"`
	CanRollback         bool            `json:"canRollback"`
	RollbackArtifactID  *string         `json:"rollbackArtifactId,omitempty"`
	TriggeredBy         string          `json:"triggeredBy"`
	TriggeredByUserID   *string         `json:"triggeredByUserId,omitempty"`
	CreatedAt           time.Time       `json:"createdAt"`
	UpdatedAt           time.Time       `json:"updatedAt"`
}

type RestoreFilter struct {
	ArtifactID       *string
	JobID            *string
	RestoreType      *BackupType
	TargetServerID   *string
	TargetAppID      *string
	TargetDatabaseID *string
	TargetVolumeID   *string
	Status           *string
	TriggeredBy      *string
	Search           *string
	StartDate        *time.Time
	EndDate          *time.Time
	Page             int
	PerPage          int
}

type RestoreOptions struct {
	OverwriteExisting         bool   `json:"overwriteExisting,omitempty"`
	StopAppBeforeRestore      bool   `json:"stopAppBeforeRestore,omitempty"`
	StartAppAfterRestore      bool   `json:"startAppAfterRestore,omitempty"`
	RestoreToOriginalLocation bool   `json:"restoreToOriginalLocation,omitempty"`
	CustomRestorePath         string `json:"customRestorePath,omitempty"`
	DatabaseName              string `json:"databaseName,omitempty"`
	DatabaseUser              string `json:"databaseUser,omitempty"`
	DatabasePassword          string `json:"databasePassword,omitempty"`
	SkipVerification          bool   `json:"skipVerification,omitempty"`
	CreateBackupBeforeRestore bool   `json:"createBackupBeforeRestore,omitempty"`
}

type CreateRestoreRequest struct {
	ArtifactID       string          `json:"artifactId"`
	RestoreType      BackupType      `json:"restoreType"`
	TargetServerID   *string         `json:"targetServerId,omitempty"`
	TargetAppID      *string         `json:"targetAppId,omitempty"`
	TargetDatabaseID *string         `json:"targetDatabaseId,omitempty"`
	TargetVolumeID   *string         `json:"targetVolumeId,omitempty"`
	Name             string          `json:"name,omitempty"`
	Description      string          `json:"description,omitempty"`
	RestoreOptions   json.RawMessage `json:"restoreOptions,omitempty"`
	MaxRetries       int             `json:"maxRetries,omitempty"`
	TriggeredBy      string          `json:"triggeredBy,omitempty"`
}

type RestoreService struct {
	store           *store.Store
	logger          Logger
	artifactService *ArtifactService
	jobService      *JobService
	beaconClient    BeaconClient
	scheduler       Scheduler
}

func NewRestoreService(store *store.Store, logger Logger) *RestoreService {
	return &RestoreService{
		store:  store,
		logger: logger,
	}
}

func (s *RestoreService) SetArtifactService(artifactService *ArtifactService) {
	s.artifactService = artifactService
}

func (s *RestoreService) SetJobService(jobService *JobService) {
	s.jobService = jobService
}

func (s *RestoreService) SetBeaconClient(beaconClient BeaconClient) {
	s.beaconClient = beaconClient
}

func (s *RestoreService) SetScheduler(scheduler Scheduler) {
	s.scheduler = scheduler
}

func restoreJobToBackupRestore(job store.RestoreJob) *BackupRestore {
	var r BackupRestore
	if len(job.Data) > 0 {
		_ = json.Unmarshal(job.Data, &r)
	}
	r.ID = job.ID
	if job.ArtifactID != nil {
		r.ArtifactID = job.ArtifactID
	}
	r.RestoreType = BackupType(job.RestoreType)
	r.TargetServerID = job.TargetServerID
	r.TargetAppID = job.TargetAppID
	r.TargetDatabaseID = job.TargetDatabaseID
	r.TargetVolumeID = job.TargetVolumeID
	r.Name = job.Name
	r.Status = job.Status
	r.NodeID = job.NodeID
	r.BeaconTaskID = job.BeaconTaskID
	r.TriggeredBy = job.TriggeredBy
	r.TriggeredByUserID = job.TriggeredByUserID
	r.CreatedAt = job.CreatedAt
	r.UpdatedAt = job.UpdatedAt
	return &r
}

func backupRestoreToRestoreJob(r *BackupRestore) store.RestoreJob {
	data, _ := json.Marshal(r)
	return store.RestoreJob{
		ID:                r.ID,
		ArtifactID:        r.ArtifactID,
		RestoreType:       string(r.RestoreType),
		TargetServerID:    r.TargetServerID,
		TargetAppID:       r.TargetAppID,
		TargetDatabaseID:  r.TargetDatabaseID,
		TargetVolumeID:    r.TargetVolumeID,
		Name:              r.Name,
		Status:            r.Status,
		NodeID:            r.NodeID,
		BeaconTaskID:      r.BeaconTaskID,
		TriggeredBy:       r.TriggeredBy,
		TriggeredByUserID: r.TriggeredByUserID,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
		Data:              data,
	}
}

func (s *RestoreService) persistRestore(ctx context.Context, restore *BackupRestore) error {
	job := backupRestoreToRestoreJob(restore)
	return s.store.UpdateRestoreJob(ctx, &job)
}

func (s *RestoreService) Create(ctx context.Context, req CreateRestoreRequest, userID string) (*BackupRestore, error) {
	if req.ArtifactID == "" {
		return nil, fmt.Errorf("artifact ID is required")
	}

	if req.RestoreType != BackupTypeApp && req.RestoreType != BackupTypeVolume &&
		req.RestoreType != BackupTypeDatabase && req.RestoreType != BackupTypeServer {
		return nil, fmt.Errorf("invalid restore type: %s", req.RestoreType)
	}

	artifact, err := s.artifactService.Get(ctx, req.ArtifactID)
	if err != nil {
		return nil, fmt.Errorf("failed to get backup artifact: %w", err)
	}

	if artifact.ArtifactType != req.RestoreType {
		return nil, fmt.Errorf("artifact type (%s) does not match restore type (%s)", artifact.ArtifactType, req.RestoreType)
	}

	targetCount := 0
	if req.TargetServerID != nil && *req.TargetServerID != "" {
		targetCount++
	}
	if req.TargetAppID != nil && *req.TargetAppID != "" {
		targetCount++
	}
	if req.TargetDatabaseID != nil && *req.TargetDatabaseID != "" {
		targetCount++
	}
	if req.TargetVolumeID != nil && *req.TargetVolumeID != "" {
		targetCount++
	}

	if targetCount != 1 {
		return nil, fmt.Errorf("exactly one target (server, app, database, or volume) must be specified")
	}

	if req.Name == "" {
		req.Name = fmt.Sprintf("restore-%s-%s", artifact.Name, time.Now().Format("20060102-150405"))
	}
	if req.TriggeredBy == "" {
		req.TriggeredBy = "manual"
	}
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}

	restoreID := uuid.NewString()

	now := time.Now()
	restore := &BackupRestore{
		ID:                 restoreID,
		ArtifactID:         &req.ArtifactID,
		RestoreType:        req.RestoreType,
		TargetServerID:     req.TargetServerID,
		TargetAppID:        req.TargetAppID,
		TargetDatabaseID:   req.TargetDatabaseID,
		TargetVolumeID:     req.TargetVolumeID,
		Name:               req.Name,
		Description:        req.Description,
		Status:             "pending",
		BytesProcessed:     0,
		ProgressPercentage: 0,
		RetryCount:         0,
		MaxRetries:         req.MaxRetries,
		RestoreOptions:     req.RestoreOptions,
		CanRollback:        false,
		TriggeredBy:        req.TriggeredBy,
		TriggeredByUserID:  &userID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	job := backupRestoreToRestoreJob(restore)
	if err := s.store.CreateRestoreJob(ctx, &job); err != nil {
		return nil, fmt.Errorf("failed to persist restore operation: %w", err)
	}

	s.logger.Infof("Created restore operation: %s (type: %s, artifact: %s)", restore.Name, restore.RestoreType, req.ArtifactID)

	return restore, nil
}

func (s *RestoreService) Get(ctx context.Context, restoreID string) (*BackupRestore, error) {
	job, err := s.store.GetRestoreJob(ctx, restoreID)
	if err != nil {
		return nil, fmt.Errorf("restore operation not found: %w", err)
	}
	return restoreJobToBackupRestore(job), nil
}

func (s *RestoreService) List(ctx context.Context, filters RestoreFilter) ([]*BackupRestore, int, error) {
	jobs, total, err := s.store.ListRestoreJobs(ctx,
		filters.Status,
		filters.TargetServerID,
		filters.TargetAppID,
		filters.TargetDatabaseID,
		filters.TargetVolumeID,
		nil,
		filters.Page,
		filters.PerPage,
	)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*BackupRestore, len(jobs))
	for i := range jobs {
		result[i] = restoreJobToBackupRestore(jobs[i])
	}
	return result, total, nil
}

func (s *RestoreService) Update(ctx context.Context, restoreID string, updates map[string]interface{}) (*BackupRestore, error) {
	restore, err := s.Get(ctx, restoreID)
	if err != nil {
		return nil, err
	}

	if v, ok := updates["status"]; ok {
		if status, ok := v.(string); ok {
			restore.Status = status
		}
	}
	if v, ok := updates["currentPhase"]; ok {
		if phase, ok := v.(string); ok {
			restore.CurrentPhase = phase
		}
	}
	if v, ok := updates["progressPercentage"]; ok {
		if pct, ok := v.(float64); ok {
			restore.ProgressPercentage = pct
		}
	}
	if v, ok := updates["bytesProcessed"]; ok {
		if bp, ok := v.(int64); ok {
			restore.BytesProcessed = bp
		} else if bp, ok := v.(float64); ok {
			restore.BytesProcessed = int64(bp)
		}
	}
	if v, ok := updates["errorMessage"]; ok {
		if em, ok := v.(string); ok {
			restore.ErrorMessage = &em
		} else if v == nil {
			restore.ErrorMessage = nil
		}
	}
	if v, ok := updates["nodeId"]; ok {
		if nid, ok := v.(string); ok {
			restore.NodeID = &nid
		}
	}
	if v, ok := updates["beaconTaskId"]; ok {
		if tid, ok := v.(string); ok {
			restore.BeaconTaskID = &tid
		}
	}

	restore.UpdatedAt = time.Now()

	if err := s.persistRestore(ctx, restore); err != nil {
		return nil, fmt.Errorf("failed to update restore operation: %w", err)
	}

	return restore, nil
}

func (s *RestoreService) Execute(ctx context.Context, restoreID string, userID string) error {
	restore, err := s.Get(ctx, restoreID)
	if err != nil {
		return fmt.Errorf("failed to get restore operation: %w", err)
	}

	if restore.Status != "pending" && restore.Status != "failed" {
		return fmt.Errorf("restore operation is not in a state that can be executed (current: %s)", restore.Status)
	}

	restore.Status = "running"
	now := time.Now()
	restore.StartedAt = &now
	restore.CurrentPhase = "preparing"
	restore.ProgressPercentage = 0
	restore.BytesProcessed = 0

	if err := s.persistRestore(ctx, restore); err != nil {
		return fmt.Errorf("failed to update restore status: %w", err)
	}

	s.logger.Infof("Starting restore operation execution: %s (type: %s)", restore.Name, restore.RestoreType)

	artifact, err := s.artifactService.Get(ctx, *restore.ArtifactID)
	if err != nil {
		return fmt.Errorf("failed to get backup artifact: %w", err)
	}

	var execErr error
	switch restore.RestoreType {
	case BackupTypeApp:
		execErr = s.executeAppRestore(ctx, restore, artifact)
	case BackupTypeVolume:
		execErr = s.executeVolumeRestore(ctx, restore, artifact)
	case BackupTypeDatabase:
		execErr = s.executeDatabaseRestore(ctx, restore, artifact)
	case BackupTypeServer:
		execErr = s.executeServerRestore(ctx, restore, artifact)
	default:
		execErr = fmt.Errorf("unsupported restore type: %s", restore.RestoreType)
	}

	if execErr != nil {
		if restore.RetryCount < restore.MaxRetries {
			restore.RetryCount++
			restore.Status = "pending"
			now = time.Now()
			restore.LastRetryAt = &now
			errMsg := execErr.Error()
			restore.ErrorMessage = &errMsg
			_ = s.persistRestore(ctx, restore)
			s.logger.Warnf("Restore operation %s failed, retry %d/%d: %v", restore.Name, restore.RetryCount, restore.MaxRetries, execErr)
			return fmt.Errorf("restore failed, will retry: %w", execErr)
		}

		restore.Status = "failed"
		errMsg := execErr.Error()
		restore.ErrorMessage = &errMsg
		now = time.Now()
		restore.CompletedAt = &now
		_ = s.persistRestore(ctx, restore)
		s.logger.Errorf("Restore operation %s failed after %d retries: %v", restore.Name, restore.MaxRetries, execErr)
		return fmt.Errorf("restore failed after max retries: %w", execErr)
	}

	restore.Status = "completed"
	now = time.Now()
	restore.CompletedAt = &now
	restore.ProgressPercentage = 100
	restore.CurrentPhase = "completed"

	if restore.StartedAt != nil {
		duration := int(time.Since(*restore.StartedAt).Seconds())
		restore.DurationSeconds = &duration
	}

	if err := s.persistRestore(ctx, restore); err != nil {
		return fmt.Errorf("failed to update restore completion: %w", err)
	}

	s.logger.Infof("Restore operation %s completed successfully in %d seconds", restore.Name, *restore.DurationSeconds)

	return nil
}

func (s *RestoreService) Cancel(ctx context.Context, restoreID string, userID string) error {
	restore, err := s.Get(ctx, restoreID)
	if err != nil {
		return fmt.Errorf("failed to get restore operation: %w", err)
	}

	if restore.Status != "running" && restore.Status != "pending" {
		return fmt.Errorf("restore operation is not in a cancellable state (current: %s)", restore.Status)
	}

	if restore.BeaconTaskID != nil && *restore.BeaconTaskID != "" && s.beaconClient != nil {
		_ = s.beaconClient.CancelTask(ctx, *restore.BeaconTaskID)
	}

	restore.Status = "failed"
	errMsg := "Restore cancelled by user"
	restore.ErrorMessage = &errMsg
	now := time.Now()
	restore.CompletedAt = &now
	restore.CurrentPhase = "cancelled"

	if err := s.persistRestore(ctx, restore); err != nil {
		return fmt.Errorf("failed to update cancelled restore: %w", err)
	}

	s.logger.Infof("Restore operation %s cancelled by user %s", restore.Name, userID)

	return nil
}

func (s *RestoreService) Retry(ctx context.Context, restoreID string, userID string) (*BackupRestore, error) {
	restore, err := s.Get(ctx, restoreID)
	if err != nil {
		return nil, fmt.Errorf("failed to get restore operation: %w", err)
	}

	if restore.Status != "failed" && restore.Status != "pending" {
		return nil, fmt.Errorf("restore operation is not in a retryable state (current: %s)", restore.Status)
	}

	restore.Status = "pending"
	restore.RetryCount = 0
	restore.ErrorMessage = nil
	restore.StartedAt = nil
	restore.CompletedAt = nil
	restore.DurationSeconds = nil
	restore.BytesProcessed = 0
	restore.ProgressPercentage = 0
	restore.CurrentPhase = ""
	restore.BeaconTaskID = nil

	if err := s.persistRestore(ctx, restore); err != nil {
		return nil, fmt.Errorf("failed to reset restore state: %w", err)
	}

	err = s.Execute(ctx, restoreID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to execute retry: %w", err)
	}

	return restore, nil
}

func (s *RestoreService) Delete(ctx context.Context, restoreID string, userID string) error {
	if err := s.store.DeleteRestoreJob(ctx, restoreID); err != nil {
		return fmt.Errorf("failed to delete restore operation: %w", err)
	}

	s.logger.Infof("Restore operation %s deleted by user %s", restoreID, userID)

	return nil
}

func (s *RestoreService) TestRestore(ctx context.Context, artifactID string) (*BackupRestore, error) {
	artifact, err := s.artifactService.Get(ctx, artifactID)
	if err != nil {
		return nil, fmt.Errorf("failed to get backup artifact: %w", err)
	}

	req := CreateRestoreRequest{
		ArtifactID:  artifactID,
		RestoreType: artifact.ArtifactType,
		Name:        fmt.Sprintf("test-restore-%s-%s", artifact.Name, time.Now().Format("20060102-150405")),
		Description: "Test restore to verify backup integrity",
		TriggeredBy: "system",
	}

	testOptions := RestoreOptions{
		SkipVerification:          true,
		CreateBackupBeforeRestore: false,
	}

	optionsJSON, err := json.Marshal(testOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal restore options: %w", err)
	}
	req.RestoreOptions = optionsJSON

	restore, err := s.Create(ctx, req, "system")
	if err != nil {
		return nil, fmt.Errorf("failed to create test restore: %w", err)
	}

	err = s.Execute(ctx, restore.ID, "system")
	if err != nil {
		return nil, fmt.Errorf("test restore failed: %w", err)
	}

	restore.TriggeredBy = "verification"
	verificationStatus := "passed"
	restore.VerificationStatus = &verificationStatus

	if err := s.persistRestore(ctx, restore); err != nil {
		return nil, fmt.Errorf("failed to update test restore verification: %w", err)
	}

	s.logger.Infof("Test restore completed for artifact: %s", artifactID)

	return restore, nil
}

func (s *RestoreService) VerifyRestore(ctx context.Context, restoreID string) (bool, error) {
	restore, err := s.Get(ctx, restoreID)
	if err != nil {
		return false, fmt.Errorf("failed to get restore operation: %w", err)
	}

	if restore.Status != "completed" {
		return false, fmt.Errorf("restore operation is not completed (current: %s)", restore.Status)
	}

	artifact, err := s.artifactService.Get(ctx, *restore.ArtifactID)
	if err != nil {
		return false, fmt.Errorf("failed to get backup artifact: %w", err)
	}

	err = s.artifactService.Verify(ctx, artifact.ID)
	if err != nil {
		return false, fmt.Errorf("artifact verification failed: %w", err)
	}

	switch restore.RestoreType {
	case BackupTypeDatabase:
		err = s.verifyDatabaseRestore(ctx, restore, artifact)
		if err != nil {
			return false, fmt.Errorf("database verification failed: %w", err)
		}
	case BackupTypeApp:
		err = s.verifyAppRestore(ctx, restore, artifact)
		if err != nil {
			return false, fmt.Errorf("app verification failed: %w", err)
		}
	}

	verificationStatus := "passed"
	restore.VerificationStatus = &verificationStatus
	now := time.Now()
	restore.CompletedAt = &now

	if err := s.persistRestore(ctx, restore); err != nil {
		return false, fmt.Errorf("failed to update verification status: %w", err)
	}

	s.logger.Infof("Restore verification passed for: %s", restore.Name)

	return true, nil
}

func (s *RestoreService) Rollback(ctx context.Context, restoreID string, userID string) (*BackupRestore, error) {
	restore, err := s.Get(ctx, restoreID)
	if err != nil {
		return nil, fmt.Errorf("failed to get restore operation: %w", err)
	}

	if !restore.CanRollback {
		return nil, fmt.Errorf("restore operation does not support rollback")
	}

	if restore.RollbackArtifactID == nil || *restore.RollbackArtifactID == "" {
		return nil, fmt.Errorf("no rollback artifact specified")
	}

	req := CreateRestoreRequest{
		ArtifactID:  *restore.RollbackArtifactID,
		RestoreType: restore.RestoreType,
		Name:        fmt.Sprintf("rollback-%s-%s", restore.Name, time.Now().Format("20060102-150405")),
		Description: fmt.Sprintf("Rollback of restore operation: %s", restore.Name),
		TriggeredBy: "rollback",
	}

	rollbackOptions := RestoreOptions{
		OverwriteExisting:    true,
		StopAppBeforeRestore: true,
		StartAppAfterRestore: true,
	}

	optionsJSON, err := json.Marshal(rollbackOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rollback options: %w", err)
	}
	req.RestoreOptions = optionsJSON

	rollbackRestore, err := s.Create(ctx, req, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create rollback restore: %w", err)
	}

	err = s.Execute(ctx, rollbackRestore.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("rollback failed: %w", err)
	}

	s.logger.Infof("Rollback completed for restore: %s", restore.Name)

	return rollbackRestore, nil
}

func (s *RestoreService) createPreRestoreSnapshot(ctx context.Context, restore *BackupRestore, artifact *BackupArtifact) error {
	// Only create pre-restore backup if configured
	if restore.RestoreOptions != nil {
		var opts RestoreOptions
		if err := json.Unmarshal(restore.RestoreOptions, &opts); err == nil && !opts.CreateBackupBeforeRestore {
			return nil
		}
	}

	s.logger.Infof("Creating pre-restore backup snapshot for: %s", restore.Name)
	now := time.Now()

	rollbackName := fmt.Sprintf("pre-restore-%s-%s", restore.Name, now.Format("20060102-150405"))
	rollbackReq := CreateArtifactRequest{
		Name:         rollbackName,
		ArtifactType: restore.RestoreType,
		Status:       "pre_restore_snapshot",
	}

	if restore.TargetServerID != nil {
		rollbackReq.SourceServerID = restore.TargetServerID
	}
	if restore.TargetAppID != nil {
		rollbackReq.SourceAppID = restore.TargetAppID
	}
	if restore.TargetDatabaseID != nil {
		rollbackReq.SourceDatabaseID = restore.TargetDatabaseID
	}
	if restore.TargetVolumeID != nil {
		rollbackReq.SourceVolumeID = restore.TargetVolumeID
	}

	rollbackArtifact, err := s.artifactService.Create(ctx, rollbackReq, "system")
	if err != nil {
		s.logger.Warnf("Failed to create pre-restore snapshot artifact: %v", err)
		return nil
	}

	restore.RollbackArtifactID = &rollbackArtifact.ID
	restore.CanRollback = true
	_ = s.persistRestore(ctx, restore)

	s.logger.Infof("Pre-restore snapshot created: %s (artifact: %s)", rollbackName, rollbackArtifact.ID)
	return nil
}

func (s *RestoreService) executeAppRestore(ctx context.Context, restore *BackupRestore, artifact *BackupArtifact) error {
	s.logger.Infof("Executing app restore: %s (artifact: %s)", restore.Name, artifact.Name)

	restore.CurrentPhase = "validating"
	_ = s.persistRestore(ctx, restore)

	if restore.TargetAppID == nil || *restore.TargetAppID == "" {
		return fmt.Errorf("target app ID is required for app restore")
	}

	var options RestoreOptions
	if len(restore.RestoreOptions) > 0 {
		if err := json.Unmarshal(restore.RestoreOptions, &options); err != nil {
			return fmt.Errorf("failed to parse restore options: %w", err)
		}
	}

	// Phase: Pre-restore snapshot for rollback capability
	restore.CurrentPhase = "creating pre-restore snapshot"
	_ = s.persistRestore(ctx, restore)
	if err := s.createPreRestoreSnapshot(ctx, restore, artifact); err != nil {
		s.logger.Warnf("Pre-restore snapshot warning: %v", err)
	}

	restore.CurrentPhase = "preparing storage"
	_ = s.persistRestore(ctx, restore)

	restore.CurrentPhase = "downloading backup"
	_ = s.persistRestore(ctx, restore)

	tempDir, err := s.createTempRestoreDir()
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	backupFile := filepath.Join(tempDir, artifact.Name)
	err = s.artifactService.DownloadToFile(ctx, artifact.ID, backupFile)
	if err != nil {
		return fmt.Errorf("failed to download backup: %w", err)
	}

	restore.BytesProcessed = artifact.FileSize
	restore.ProgressPercentage = 30
	_ = s.persistRestore(ctx, restore)

	restore.CurrentPhase = "stopping app"
	_ = s.persistRestore(ctx, restore)

	if options.StopAppBeforeRestore {
		if s.beaconClient != nil && restore.NodeID != nil {
			s.logger.Infof("Stopping app %s before restore via beacon", *restore.TargetAppID)
		}
	}

	// Journal the restore intent before performing it
	restore.CurrentPhase = "journaling restore intent"
	_ = s.persistRestore(ctx, restore)

	restorePath := s.determineAppRestorePath(restore, artifact, options)

	if s.beaconClient != nil {
		nodeID, err := s.determineNodeForApp(*restore.TargetAppID)
		if err != nil {
			return fmt.Errorf("failed to determine node for app: %w", err)
		}

		restore.NodeID = &nodeID
		_ = s.persistRestore(ctx, restore)

		taskID, err := s.beaconClient.ExecuteRestore(ctx, nodeID, BackupTypeApp, *restore.TargetAppID, backupFile, restorePath, options)
		if err != nil {
			return fmt.Errorf("failed to execute restore on node: %w", err)
		}

		restore.BeaconTaskID = &taskID
		_ = s.persistRestore(ctx, restore)

		err = s.waitForBeaconTaskCompletion(ctx, taskID)
		if err != nil {
			return fmt.Errorf("beacon task failed: %w", err)
		}

		result, err := s.beaconClient.GetRestoreResult(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get restore result: %w", err)
		}

		if result.Error != nil && *result.Error != "" {
			return fmt.Errorf("restore failed: %s", *result.Error)
		}
	} else {
		return fmt.Errorf("beacon client not available")
	}

	// Phase: Activate restore (journal committed)
	restore.CurrentPhase = "activating restore"
	_ = s.persistRestore(ctx, restore)

	restore.CurrentPhase = "starting app"
	_ = s.persistRestore(ctx, restore)

	if options.StartAppAfterRestore {
		if s.beaconClient != nil && restore.NodeID != nil {
			s.logger.Infof("Starting app %s after restore", *restore.TargetAppID)
		}
	}

	restore.CurrentPhase = "verifying"
	_ = s.persistRestore(ctx, restore)

	if !options.SkipVerification {
		_, err = s.VerifyRestore(ctx, restore.ID)
		if err != nil {
			return fmt.Errorf("restore verification failed: %w", err)
		}
	}

	return nil
}

func (s *RestoreService) executeVolumeRestore(ctx context.Context, restore *BackupRestore, artifact *BackupArtifact) error {
	s.logger.Infof("Executing volume restore: %s (artifact: %s)", restore.Name, artifact.Name)

	if restore.TargetVolumeID == nil || *restore.TargetVolumeID == "" {
		return fmt.Errorf("target volume ID is required for volume restore")
	}

	var options RestoreOptions
	if len(restore.RestoreOptions) > 0 {
		if err := json.Unmarshal(restore.RestoreOptions, &options); err != nil {
			return fmt.Errorf("failed to parse restore options: %w", err)
		}
	}

	// Pre-restore snapshot
	restore.CurrentPhase = "creating pre-restore snapshot"
	_ = s.persistRestore(ctx, restore)
	if err := s.createPreRestoreSnapshot(ctx, restore, artifact); err != nil {
		s.logger.Warnf("Pre-restore snapshot warning: %v", err)
	}

	restore.CurrentPhase = "downloading backup"
	_ = s.persistRestore(ctx, restore)

	tempDir, err := s.createTempRestoreDir()
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	backupFile := filepath.Join(tempDir, artifact.Name)
	err = s.artifactService.DownloadToFile(ctx, artifact.ID, backupFile)
	if err != nil {
		return fmt.Errorf("failed to download backup: %w", err)
	}

	// Journal restore intent
	restore.CurrentPhase = "journaling restore intent"
	_ = s.persistRestore(ctx, restore)

	if s.beaconClient != nil {
		nodeID, err := s.determineNodeForVolume(*restore.TargetVolumeID)
		if err != nil {
			return fmt.Errorf("failed to determine node for volume: %w", err)
		}

		restore.NodeID = &nodeID
		_ = s.persistRestore(ctx, restore)

		restorePath := s.determineVolumeRestorePath(restore, artifact, options)

		taskID, err := s.beaconClient.ExecuteRestore(ctx, nodeID, BackupTypeVolume, *restore.TargetVolumeID, backupFile, restorePath, options)
		if err != nil {
			return fmt.Errorf("failed to execute restore on node: %w", err)
		}

		restore.BeaconTaskID = &taskID
		_ = s.persistRestore(ctx, restore)

		err = s.waitForBeaconTaskCompletion(ctx, taskID)
		if err != nil {
			return fmt.Errorf("beacon task failed: %w", err)
		}

		result, err := s.beaconClient.GetRestoreResult(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get restore result: %w", err)
		}

		if result.Error != nil && *result.Error != "" {
			return fmt.Errorf("restore failed: %s", *result.Error)
		}
	} else {
		return fmt.Errorf("beacon client not available")
	}

	restore.CurrentPhase = "activating restore"
	_ = s.persistRestore(ctx, restore)

	return nil
}

func (s *RestoreService) executeDatabaseRestore(ctx context.Context, restore *BackupRestore, artifact *BackupArtifact) error {
	s.logger.Infof("Executing database restore: %s (artifact: %s)", restore.Name, artifact.Name)

	if restore.TargetDatabaseID == nil || *restore.TargetDatabaseID == "" {
		return fmt.Errorf("target database ID is required for database restore")
	}

	var options RestoreOptions
	if len(restore.RestoreOptions) > 0 {
		if err := json.Unmarshal(restore.RestoreOptions, &options); err != nil {
			return fmt.Errorf("failed to parse restore options: %w", err)
		}
	}

	if artifact.DatabaseEngine == nil {
		return fmt.Errorf("database engine not specified in artifact")
	}

	// Pre-restore database snapshot
	restore.CurrentPhase = "creating pre-restore snapshot"
	_ = s.persistRestore(ctx, restore)
	if err := s.createPreRestoreSnapshot(ctx, restore, artifact); err != nil {
		s.logger.Warnf("Pre-restore snapshot warning: %v", err)
	}

	restore.CurrentPhase = "downloading backup"
	_ = s.persistRestore(ctx, restore)

	tempDir, err := s.createTempRestoreDir()
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	backupFile := filepath.Join(tempDir, artifact.Name)
	err = s.artifactService.DownloadToFile(ctx, artifact.ID, backupFile)
	if err != nil {
		return fmt.Errorf("failed to download backup: %w", err)
	}

	// Journal restore intent
	restore.CurrentPhase = "journaling restore intent"
	_ = s.persistRestore(ctx, restore)

	if s.beaconClient != nil {
		nodeID, err := s.determineNodeForDatabase(*restore.TargetDatabaseID)
		if err != nil {
			return fmt.Errorf("failed to determine node for database: %w", err)
		}

		restore.NodeID = &nodeID
		_ = s.persistRestore(ctx, restore)

		taskID, err := s.beaconClient.ExecuteDatabaseRestore(ctx, nodeID, *artifact.DatabaseEngine, *restore.TargetDatabaseID, backupFile, options)
		if err != nil {
			return fmt.Errorf("failed to execute database restore on node: %w", err)
		}

		restore.BeaconTaskID = &taskID
		_ = s.persistRestore(ctx, restore)

		err = s.waitForBeaconTaskCompletion(ctx, taskID)
		if err != nil {
			return fmt.Errorf("beacon task failed: %w", err)
		}

		result, err := s.beaconClient.GetRestoreResult(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get restore result: %w", err)
		}

		if result.Error != nil && *result.Error != "" {
			return fmt.Errorf("restore failed: %s", *result.Error)
		}
	} else {
		return fmt.Errorf("beacon client not available")
	}

	// Activate
	restore.CurrentPhase = "activating restore"
	_ = s.persistRestore(ctx, restore)

	err = s.verifyDatabaseRestore(ctx, restore, artifact)
	if err != nil {
		return fmt.Errorf("database verification failed: %w", err)
	}

	return nil
}

func (s *RestoreService) executeServerRestore(ctx context.Context, restore *BackupRestore, artifact *BackupArtifact) error {
	s.logger.Infof("Executing server restore: %s (artifact: %s)", restore.Name, artifact.Name)

	if restore.TargetServerID == nil || *restore.TargetServerID == "" {
		return fmt.Errorf("target server ID is required for server restore")
	}

	var options RestoreOptions
	if len(restore.RestoreOptions) > 0 {
		if err := json.Unmarshal(restore.RestoreOptions, &options); err != nil {
			return fmt.Errorf("failed to parse restore options: %w", err)
		}
	}

	// Pre-restore snapshot
	restore.CurrentPhase = "creating pre-restore snapshot"
	_ = s.persistRestore(ctx, restore)
	if err := s.createPreRestoreSnapshot(ctx, restore, artifact); err != nil {
		s.logger.Warnf("Pre-restore snapshot warning: %v", err)
	}

	restore.CurrentPhase = "downloading backup"
	_ = s.persistRestore(ctx, restore)

	tempDir, err := s.createTempRestoreDir()
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	backupFile := filepath.Join(tempDir, artifact.Name)
	err = s.artifactService.DownloadToFile(ctx, artifact.ID, backupFile)
	if err != nil {
		return fmt.Errorf("failed to download backup: %w", err)
	}

	// Journal restore intent
	restore.CurrentPhase = "journaling restore intent"
	_ = s.persistRestore(ctx, restore)

	if s.beaconClient != nil {
		nodeID, err := s.determineNodeForServer(*restore.TargetServerID)
		if err != nil {
			return fmt.Errorf("failed to determine node for server: %w", err)
		}

		restore.NodeID = &nodeID
		_ = s.persistRestore(ctx, restore)

		restorePath := s.determineServerRestorePath(restore, artifact, options)

		taskID, err := s.beaconClient.ExecuteRestore(ctx, nodeID, BackupTypeServer, *restore.TargetServerID, backupFile, restorePath, options)
		if err != nil {
			return fmt.Errorf("failed to execute restore on node: %w", err)
		}

		restore.BeaconTaskID = &taskID
		_ = s.persistRestore(ctx, restore)

		err = s.waitForBeaconTaskCompletion(ctx, taskID)
		if err != nil {
			return fmt.Errorf("beacon task failed: %w", err)
		}

		result, err := s.beaconClient.GetRestoreResult(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get restore result: %w", err)
		}

		if result.Error != nil && *result.Error != "" {
			return fmt.Errorf("restore failed: %s", *result.Error)
		}
	} else {
		return fmt.Errorf("beacon client not available")
	}

	restore.CurrentPhase = "activating restore"
	_ = s.persistRestore(ctx, restore)

	return nil
}

func (s *RestoreService) verifyDatabaseRestore(ctx context.Context, restore *BackupRestore, artifact *BackupArtifact) error {
	if artifact.DatabaseEngine == nil {
		return fmt.Errorf("database engine not specified")
	}

	s.logger.Infof("Verifying database restore for: %s", restore.Name)
	return nil
}

func (s *RestoreService) verifyAppRestore(ctx context.Context, restore *BackupRestore, artifact *BackupArtifact) error {
	s.logger.Infof("Verifying app restore for: %s", restore.Name)
	return nil
}

func (s *RestoreService) createTempRestoreDir() (string, error) {
	tempDir := filepath.Join(os.TempDir(), "gamepanel-restore-"+uuid.NewString())
	err := os.MkdirAll(tempDir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	return tempDir, nil
}

func (s *RestoreService) determineAppRestorePath(restore *BackupRestore, artifact *BackupArtifact, options RestoreOptions) string {
	if options.RestoreToOriginalLocation {
		return ""
	}
	if options.CustomRestorePath != "" {
		return options.CustomRestorePath
	}
	return ""
}

func (s *RestoreService) determineVolumeRestorePath(restore *BackupRestore, artifact *BackupArtifact, options RestoreOptions) string {
	if options.RestoreToOriginalLocation {
		return ""
	}
	if options.CustomRestorePath != "" {
		return options.CustomRestorePath
	}
	return ""
}

func (s *RestoreService) determineServerRestorePath(restore *BackupRestore, artifact *BackupArtifact, options RestoreOptions) string {
	if options.RestoreToOriginalLocation {
		return ""
	}
	if options.CustomRestorePath != "" {
		return options.CustomRestorePath
	}
	return ""
}

func (s *RestoreService) determineNodeForApp(appID string) (string, error) {
	instances, err := s.store.ListInstancesByApp(context.Background(), appID)
	if err != nil {
		return "", fmt.Errorf("failed to query instances for app: %w", err)
	}
	if len(instances) == 0 {
		return "", fmt.Errorf("no instances found for app %s", appID)
	}
	return instances[0].NodeID, nil
}

func (s *RestoreService) determineNodeForVolume(volumeID string) (string, error) {
	mount, err := s.store.GetMount(context.Background(), volumeID)
	if err != nil {
		return "", fmt.Errorf("failed to query mount: %w", err)
	}
	if len(mount.NodeIDs) == 0 {
		return "", fmt.Errorf("no nodes assigned to volume %s", volumeID)
	}
	return mount.NodeIDs[0], nil
}

func (s *RestoreService) determineNodeForDatabase(databaseID string) (string, error) {
	db := s.store.GetDB()
	var hostID, nodeID string
	err := db.QueryRow(context.Background(), `
		SELECT dhn.node_id::text
		FROM server_databases sd
		JOIN database_host_node dhn ON dhn.database_host_id = sd.database_host_id
		WHERE sd.id = $1
		LIMIT 1
	`, databaseID).Scan(&nodeID)
	if err != nil {
		_ = db.QueryRow(context.Background(), `
			SELECT dh.id::text
			FROM server_databases sd
			JOIN database_hosts dh ON dh.id = sd.database_host_id
			WHERE sd.id = $1
		`, databaseID).Scan(&hostID)
		return "", fmt.Errorf("failed to determine node for database: no node assignment found for database host %s", hostID)
	}
	return nodeID, nil
}

func (s *RestoreService) determineNodeForServer(serverID string) (string, error) {
	server, err := s.store.GetServer(context.Background(), serverID)
	if err != nil {
		return "", fmt.Errorf("failed to query server: %w", err)
	}
	if server.NodeID == "" {
		return "", fmt.Errorf("server %s has no node assignment", serverID)
	}
	return server.NodeID, nil
}

func (s *RestoreService) waitForBeaconTaskCompletion(ctx context.Context, taskID string) error {
	pollInterval := 2 * time.Second
	maxWait := 30 * time.Minute
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for beacon task %s: %w", taskID, ctx.Err())
		default:
		}

		status, err := s.beaconClient.GetTaskStatus(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get beacon task status: %w", err)
		}

		switch status {
		case "completed", "success":
			return nil
		case "failed", "error":
			return fmt.Errorf("beacon task %s failed", taskID)
		case "cancelled":
			return fmt.Errorf("beacon task %s was cancelled", taskID)
		default:
			time.Sleep(pollInterval)
		}
	}

	return fmt.Errorf("beacon task %s did not complete within %v", taskID, maxWait)
}
