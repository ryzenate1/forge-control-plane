package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

// BackupArtifact represents a backup artifact (actual backup file)
type BackupArtifact struct {
	ID                   string          `json:"id"`
	JobID                *string         `json:"jobId,omitempty"`
	ConfigurationID      *string         `json:"configurationId,omitempty"`
	ArtifactType         BackupType      `json:"artifactType"`
	Name                 string          `json:"name"`
	DisplayName          string          `json:"displayName,omitempty"`
	StorageProvider      string          `json:"storageProvider"`
	StoragePath          string          `json:"storagePath"`
	StorageURL           *string         `json:"storageUrl,omitempty"`
	FileSize             int64           `json:"fileSize"`
	FileHash             *string         `json:"fileHash,omitempty"`
	HashAlgorithm        string          `json:"hashAlgorithm"`
	SourceServerID       *string         `json:"sourceServerId,omitempty"`
	SourceAppID          *string         `json:"sourceAppId,omitempty"`
	SourceDatabaseID     *string         `json:"sourceDatabaseId,omitempty"`
	SourceVolumeID       *string         `json:"sourceVolumeId,omitempty"`
	DatabaseEngine       *DatabaseEngine `json:"databaseEngine,omitempty"`
	DatabaseName         *string         `json:"databaseName,omitempty"`
	VolumeName           *string         `json:"volumeName,omitempty"`
	VolumeMountPath      *string         `json:"volumeMountPath,omitempty"`
	AppName              *string         `json:"appName,omitempty"`
	AppVersion           *string         `json:"appVersion,omitempty"`
	IsCompressed         bool            `json:"isCompressed"`
	CompressionAlgorithm *string         `json:"compressionAlgorithm,omitempty"`
	IsEncrypted          bool            `json:"isEncrypted"`
	EncryptionAlgorithm  *string         `json:"encryptionAlgorithm,omitempty"`
	Status               string          `json:"status"`
	IsVerified           bool            `json:"isVerified"`
	VerificationAttempts int             `json:"verificationAttempts"`
	LastVerifiedAt       *time.Time      `json:"lastVerifiedAt,omitempty"`
	IsLocked             bool            `json:"isLocked"`
	LockReason           *string         `json:"lockReason,omitempty"`
	ExpiresAt            *time.Time      `json:"expiresAt,omitempty"`
	Manifest             json.RawMessage `json:"manifest,omitempty"`
	Metadata             json.RawMessage `json:"metadata,omitempty"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
	UploadedAt           *time.Time      `json:"uploadedAt,omitempty"`
}

// ArtifactFilter represents filters for listing backup artifacts
type ArtifactFilter struct {
	JobID            *string
	ConfigurationID  *string
	ArtifactType     *BackupType
	SourceServerID   *string
	SourceAppID      *string
	SourceDatabaseID *string
	SourceVolumeID   *string
	StorageProvider  *string
	Status           *string
	IsLocked         *bool
	Search           *string
	StartDate        *time.Time
	EndDate          *time.Time
	Page             int
	PerPage          int
}

// BackupResult represents the result of a backup operation from beacon
type BackupResult struct {
	ArtifactName         string          `json:"artifactName"`
	StoragePath          string          `json:"storagePath"`
	StorageURL           string          `json:"storageUrl,omitempty"`
	FileSize             int64           `json:"fileSize"`
	FileHash             string          `json:"fileHash"`
	HashAlgorithm        string          `json:"hashAlgorithm"`
	SourceType           string          `json:"sourceType"`
	SourceID             string          `json:"sourceId"`
	DatabaseEngine       *DatabaseEngine `json:"databaseEngine,omitempty"`
	DatabaseName         *string         `json:"databaseName,omitempty"`
	VolumeName           *string         `json:"volumeName,omitempty"`
	VolumeMountPath      *string         `json:"volumeMountPath,omitempty"`
	AppName              *string         `json:"appName,omitempty"`
	AppVersion           *string         `json:"appVersion,omitempty"`
	IsCompressed         bool            `json:"isCompressed"`
	CompressionAlgorithm *string         `json:"compressionAlgorithm,omitempty"`
	IsEncrypted          bool            `json:"isEncrypted"`
	EncryptionAlgorithm  *string         `json:"encryptionAlgorithm,omitempty"`
	Manifest             json.RawMessage `json:"manifest,omitempty"`
	Metadata             json.RawMessage `json:"metadata,omitempty"`
	Error                *string         `json:"error,omitempty"`
}

// ArtifactService handles backup artifact management
type ArtifactService struct {
	store           *store.Store
	logger          Logger
	storageAdapters map[string]StorageAdapter
	defaultAdapter  StorageAdapter
	daemonClient    *daemon.Client
}

func (s *ArtifactService) SetDaemonClient(client *daemon.Client) {
	s.daemonClient = client
}

// NewArtifactService creates a new ArtifactService
func NewArtifactService(store *store.Store, logger Logger) *ArtifactService {
	return &ArtifactService{
		store:           store,
		logger:          logger,
		storageAdapters: make(map[string]StorageAdapter),
	}
}

// RegisterStorageAdapter registers a storage adapter
func (s *ArtifactService) RegisterStorageAdapter(provider string, adapter StorageAdapter) {
	s.storageAdapters[provider] = adapter
	if s.defaultAdapter == nil {
		s.defaultAdapter = adapter
	}
}

// SetDefaultAdapter sets the default storage adapter
func (s *ArtifactService) SetDefaultAdapter(adapter StorageAdapter) {
	s.defaultAdapter = adapter
}

// Create creates a new backup artifact
func (s *ArtifactService) Create(ctx context.Context, req CreateArtifactRequest, userID string) (*BackupArtifact, error) {
	// Validate the request
	if req.Name == "" {
		return nil, fmt.Errorf("artifact name is required")
	}

	if req.ArtifactType != BackupTypeApp && req.ArtifactType != BackupTypeVolume &&
		req.ArtifactType != BackupTypeDatabase && req.ArtifactType != BackupTypeServer {
		return nil, fmt.Errorf("invalid artifact type: %s", req.ArtifactType)
	}

	// Validate that exactly one source is specified
	sourceCount := 0
	if req.SourceServerID != nil && *req.SourceServerID != "" {
		sourceCount++
	}
	if req.SourceAppID != nil && *req.SourceAppID != "" {
		sourceCount++
	}
	if req.SourceDatabaseID != nil && *req.SourceDatabaseID != "" {
		sourceCount++
	}
	if req.SourceVolumeID != nil && *req.SourceVolumeID != "" {
		sourceCount++
	}

	if sourceCount != 1 {
		return nil, fmt.Errorf("exactly one source (server, app, database, or volume) must be specified")
	}

	// Set defaults
	if req.StorageProvider == "" {
		req.StorageProvider = "local"
	}
	if req.HashAlgorithm == "" {
		req.HashAlgorithm = "sha256"
	}
	if req.Status == "" {
		req.Status = "created"
	}

	// Generate ID
	artifactID := uuid.NewString()

	now := time.Now()
	artifact := &BackupArtifact{
		ID:                   artifactID,
		JobID:                req.JobID,
		ConfigurationID:      req.ConfigurationID,
		ArtifactType:         req.ArtifactType,
		Name:                 req.Name,
		DisplayName:          req.DisplayName,
		StorageProvider:      req.StorageProvider,
		StoragePath:          req.StoragePath,
		StorageURL:           req.StorageURL,
		FileSize:             req.FileSize,
		FileHash:             req.FileHash,
		HashAlgorithm:        req.HashAlgorithm,
		SourceServerID:       req.SourceServerID,
		SourceAppID:          req.SourceAppID,
		SourceDatabaseID:     req.SourceDatabaseID,
		SourceVolumeID:       req.SourceVolumeID,
		DatabaseEngine:       req.DatabaseEngine,
		DatabaseName:         req.DatabaseName,
		VolumeName:           req.VolumeName,
		VolumeMountPath:      req.VolumeMountPath,
		AppName:              req.AppName,
		AppVersion:           req.AppVersion,
		IsCompressed:         req.IsCompressed,
		CompressionAlgorithm: req.CompressionAlgorithm,
		IsEncrypted:          req.IsEncrypted,
		EncryptionAlgorithm:  req.EncryptionAlgorithm,
		Status:               req.Status,
		IsVerified:           req.IsVerified,
		IsLocked:             req.IsLocked,
		LockReason:           req.LockReason,
		ExpiresAt:            req.ExpiresAt,
		Manifest:             req.Manifest,
		Metadata:             req.Metadata,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	// Calculate expiration if retention days are specified
	if req.RetentionDays > 0 {
		expiresAt := now.Add(time.Duration(req.RetentionDays) * 24 * time.Hour)
		artifact.ExpiresAt = &expiresAt
	}
	record, err := backupArtifactToRecord(artifact)
	if err != nil {
		return nil, err
	}
	if err := s.store.CreateBackupArtifactRecord(ctx, &record); err != nil {
		return nil, fmt.Errorf("persist backup artifact: %w", err)
	}
	artifact.CreatedAt = record.CreatedAt
	artifact.UpdatedAt = record.UpdatedAt
	artifact.UploadedAt = record.UploadedAt

	s.logger.Infof("Created backup artifact: %s (type: %s, size: %d bytes)", artifact.Name, artifact.ArtifactType, artifact.FileSize)

	return artifact, nil
}

// CreateFromBackupResult creates a backup artifact from a beacon backup result
func (s *ArtifactService) CreateFromBackupResult(ctx context.Context, job *BackupJob, result *BackupResult) (*BackupArtifact, error) {
	if result == nil {
		return nil, fmt.Errorf("backup result is required")
	}

	if result.Error != nil && *result.Error != "" {
		return nil, fmt.Errorf("backup result contains error: %s", *result.Error)
	}

	// Determine artifact type from job type
	artifactType := job.JobType

	// Create the artifact
	req := CreateArtifactRequest{
		JobID:                &job.ID,
		ArtifactType:         artifactType,
		Name:                 result.ArtifactName,
		DisplayName:          fmt.Sprintf("%s - %s", job.Name, time.Now().Format("2006-01-02 15:04:05")),
		StorageProvider:      job.StorageProvider,
		StoragePath:          result.StoragePath,
		StorageURL:           stringPtr(result.StorageURL),
		FileSize:             result.FileSize,
		FileHash:             stringPtr(result.FileHash),
		HashAlgorithm:        result.HashAlgorithm,
		IsCompressed:         result.IsCompressed,
		CompressionAlgorithm: result.CompressionAlgorithm,
		IsEncrypted:          result.IsEncrypted,
		EncryptionAlgorithm:  result.EncryptionAlgorithm,
		Status:               "uploaded",
		Manifest:             result.Manifest,
		Metadata:             result.Metadata,
	}

	// Set source based on job type
	switch job.JobType {
	case BackupTypeApp:
		if job.AppID != nil {
			req.SourceAppID = job.AppID
		}
		if result.AppName != nil {
			req.AppName = result.AppName
		}
		if result.AppVersion != nil {
			req.AppVersion = result.AppVersion
		}
	case BackupTypeVolume:
		if job.VolumeID != nil {
			req.SourceVolumeID = job.VolumeID
		}
		if result.VolumeName != nil {
			req.VolumeName = result.VolumeName
		}
		if result.VolumeMountPath != nil {
			req.VolumeMountPath = result.VolumeMountPath
		}
	case BackupTypeDatabase:
		if job.DatabaseID != nil {
			req.SourceDatabaseID = job.DatabaseID
		}
		if result.DatabaseEngine != nil {
			req.DatabaseEngine = result.DatabaseEngine
		}
		if result.DatabaseName != nil {
			req.DatabaseName = result.DatabaseName
		}
	case BackupTypeServer:
		if job.ServerID != nil {
			req.SourceServerID = job.ServerID
		}
	}

	artifact, err := s.Create(ctx, req, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create artifact: %w", err)
	}

	// Set uploaded timestamp
	now := time.Now()
	artifact.UploadedAt = &now
	if err := s.persistArtifact(ctx, artifact); err != nil {
		return nil, err
	}

	s.logger.Infof("Created backup artifact from result: %s (job: %s)", artifact.Name, job.ID)

	return artifact, nil
}

// Get retrieves a backup artifact by ID
func (s *ArtifactService) Get(ctx context.Context, artifactID string) (*BackupArtifact, error) {
	record, err := s.store.GetBackupArtifactRecord(ctx, artifactID)
	if err != nil {
		return nil, fmt.Errorf("get backup artifact: %w", err)
	}
	return backupArtifactFromRecord(record), nil
}

// List retrieves backup artifacts with optional filtering
func (s *ArtifactService) List(ctx context.Context, filters ArtifactFilter) ([]*BackupArtifact, int, error) {
	records, err := s.store.ListBackupArtifactRecords(ctx)
	if err != nil {
		return nil, 0, err
	}
	artifacts := make([]*BackupArtifact, 0, len(records))
	for _, record := range records {
		artifact := backupArtifactFromRecord(record)
		if filters.JobID != nil && !sameStringPointer(artifact.JobID, filters.JobID) {
			continue
		}
		if filters.ConfigurationID != nil && !sameStringPointer(artifact.ConfigurationID, filters.ConfigurationID) {
			continue
		}
		if filters.ArtifactType != nil && artifact.ArtifactType != *filters.ArtifactType {
			continue
		}
		if filters.SourceServerID != nil && !sameStringPointer(artifact.SourceServerID, filters.SourceServerID) {
			continue
		}
		if filters.SourceAppID != nil && !sameStringPointer(artifact.SourceAppID, filters.SourceAppID) {
			continue
		}
		if filters.SourceDatabaseID != nil && !sameStringPointer(artifact.SourceDatabaseID, filters.SourceDatabaseID) {
			continue
		}
		if filters.SourceVolumeID != nil && !sameStringPointer(artifact.SourceVolumeID, filters.SourceVolumeID) {
			continue
		}
		if filters.StorageProvider != nil && artifact.StorageProvider != *filters.StorageProvider {
			continue
		}
		if filters.Status != nil && artifact.Status != *filters.Status {
			continue
		}
		if filters.IsLocked != nil && artifact.IsLocked != *filters.IsLocked {
			continue
		}
		if filters.Search != nil {
			query := strings.ToLower(strings.TrimSpace(*filters.Search))
			if query != "" && !strings.Contains(strings.ToLower(artifact.Name+" "+artifact.DisplayName), query) {
				continue
			}
		}
		if filters.StartDate != nil && artifact.CreatedAt.Before(*filters.StartDate) {
			continue
		}
		if filters.EndDate != nil && artifact.CreatedAt.After(*filters.EndDate) {
			continue
		}
		artifacts = append(artifacts, artifact)
	}
	total := len(artifacts)
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
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return artifacts[start:end], total, nil
}

// Update updates a backup artifact
func (s *ArtifactService) Update(ctx context.Context, artifactID string, updates map[string]interface{}) (*BackupArtifact, error) {
	artifact, err := s.Get(ctx, artifactID)
	if err != nil {
		return nil, err
	}
	if value, ok := updates["status"].(string); ok {
		artifact.Status = value
	}
	if value, ok := updates["isVerified"].(bool); ok {
		artifact.IsVerified = value
	}
	if value, ok := updates["fileSize"].(int64); ok {
		artifact.FileSize = value
	}
	if value, ok := updates["fileHash"].(string); ok {
		artifact.FileHash = &value
	}
	if err := s.persistArtifact(ctx, artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

// Delete deletes a backup artifact
func (s *ArtifactService) Delete(ctx context.Context, artifactID string, userID string) error {
	artifact, err := s.Get(ctx, artifactID)
	if err != nil {
		return fmt.Errorf("failed to get backup artifact: %w", err)
	}

	if artifact.IsLocked {
		reason := "unknown"
		if artifact.LockReason != nil && *artifact.LockReason != "" {
			reason = *artifact.LockReason
		}
		return fmt.Errorf("backup artifact is locked and cannot be deleted (reason: %s)", reason)
	}

	// Delete from storage
	if err := s.deleteFromStorage(ctx, artifact); err != nil {
		return fmt.Errorf("delete artifact from storage: %w", err)
	}
	if err := s.store.DeleteBackupArtifactRecord(ctx, artifactID); err != nil {
		return fmt.Errorf("delete backup artifact record: %w", err)
	}
	s.logger.Infof("Deleted backup artifact: %s", artifact.Name)

	return nil
}

// Lock locks a backup artifact
func (s *ArtifactService) Lock(ctx context.Context, artifactID string, userID string, reason string) error {
	artifact, err := s.Get(ctx, artifactID)
	if err != nil {
		return fmt.Errorf("failed to get backup artifact: %w", err)
	}

	if artifact.IsLocked {
		return fmt.Errorf("backup artifact is already locked")
	}

	artifact.IsLocked = true
	artifact.LockReason = &reason
	if err := s.persistArtifact(ctx, artifact); err != nil {
		return err
	}

	s.logger.Infof("Locked backup artifact: %s (reason: %s)", artifact.Name, reason)

	return nil
}

// Unlock unlocks a backup artifact
func (s *ArtifactService) Unlock(ctx context.Context, artifactID string, userID string) error {
	artifact, err := s.Get(ctx, artifactID)
	if err != nil {
		return fmt.Errorf("failed to get backup artifact: %w", err)
	}

	if !artifact.IsLocked {
		return fmt.Errorf("backup artifact is not locked")
	}

	artifact.IsLocked = false
	artifact.LockReason = nil
	if err := s.persistArtifact(ctx, artifact); err != nil {
		return err
	}

	s.logger.Infof("Unlocked backup artifact: %s", artifact.Name)

	return nil
}

// Verify verifies a backup artifact's integrity
func (s *ArtifactService) Verify(ctx context.Context, artifactID string) error {
	artifact, err := s.Get(ctx, artifactID)
	if err != nil {
		return fmt.Errorf("failed to get backup artifact: %w", err)
	}

	if artifact.IsVerified {
		return nil // Already verified
	}

	s.logger.Infof("Verifying backup artifact: %s", artifact.Name)

	var calculatedHash string
	if artifact.StorageProvider == "beacon" {
		reader, err := s.downloadFromBeacon(ctx, artifact)
		if err != nil {
			return fmt.Errorf("download beacon artifact for verification: %w", err)
		}
		defer reader.Close()
		hasher := sha256.New()
		if _, err := io.Copy(hasher, reader); err != nil {
			return fmt.Errorf("hash beacon artifact: %w", err)
		}
		calculatedHash = hex.EncodeToString(hasher.Sum(nil))
	} else {
		adapter, err := s.getStorageAdapter(artifact.StorageProvider)
		if err != nil {
			return fmt.Errorf("failed to get storage adapter: %w", err)
		}
		tempFile, err := s.downloadToTempFile(ctx, adapter, artifact.StoragePath)
		if err != nil {
			return fmt.Errorf("failed to download artifact for verification: %w", err)
		}
		defer os.Remove(tempFile)
		calculatedHash, err = calculateFileHash(tempFile, artifact.HashAlgorithm)
		if err != nil {
			return fmt.Errorf("failed to calculate file hash: %w", err)
		}
	}

	// Compare with stored hash
	if artifact.FileHash == nil || *artifact.FileHash == "" {
		// No stored hash, just mark as verified
		s.logger.Warnf("No stored hash for artifact %s, skipping hash verification", artifact.Name)
	} else if *artifact.FileHash != calculatedHash {
		artifact.VerificationAttempts++
		_ = s.persistArtifact(ctx, artifact)
		return fmt.Errorf("hash mismatch: expected %s, got %s", *artifact.FileHash, calculatedHash)
	}

	// Mark as verified
	artifact.IsVerified = true
	artifact.VerificationAttempts = 0
	now := time.Now()
	artifact.LastVerifiedAt = &now
	artifact.Status = "verified"
	if err := s.persistArtifact(ctx, artifact); err != nil {
		return err
	}

	s.logger.Infof("Backup artifact verified: %s", artifact.Name)

	return nil
}

// VerifyChecksum verifies the checksum of a backup artifact
func (s *ArtifactService) VerifyChecksum(ctx context.Context, artifactID string) (bool, error) {
	err := s.Verify(ctx, artifactID)
	if err != nil {
		return false, err
	}
	return true, nil
}

// Download downloads a backup artifact
func (s *ArtifactService) Download(ctx context.Context, artifactID string) (io.Reader, error) {
	artifact, err := s.Get(ctx, artifactID)
	if err != nil {
		return nil, fmt.Errorf("failed to get backup artifact: %w", err)
	}

	if artifact.StorageProvider == "beacon" {
		return s.downloadFromBeacon(ctx, artifact)
	}

	adapter, err := s.getStorageAdapter(artifact.StorageProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage adapter: %w", err)
	}

	// Download the artifact
	data, err := adapter.Download(ctx, artifact.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("failed to download artifact: %w", err)
	}

	return newBytesReader(data), nil
}

// DownloadToFile downloads a backup artifact to a file
func (s *ArtifactService) DownloadToFile(ctx context.Context, artifactID string, destinationPath string) error {
	reader, err := s.Download(ctx, artifactID)
	if err != nil {
		return fmt.Errorf("failed to download artifact: %w", err)
	}
	if br, ok := reader.(*bytesReader); ok {
		defer br.Close()
	} else {
		return fmt.Errorf("unexpected reader type: %T", reader)
	}

	// Create destination directory
	dir := filepath.Dir(destinationPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Create the file
	file, err := os.Create(destinationPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer file.Close()

	// Copy the data
	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to copy artifact data: %w", err)
	}

	return nil
}

// GetManifest retrieves the manifest for a backup artifact
func (s *ArtifactService) GetManifest(ctx context.Context, artifactID string) (*BackupManifest, error) {
	artifact, err := s.Get(ctx, artifactID)
	if err != nil {
		return nil, fmt.Errorf("failed to get backup artifact: %w", err)
	}

	if len(artifact.Manifest) == 0 {
		return nil, fmt.Errorf("artifact has no manifest")
	}

	var manifest BackupManifest
	err = json.Unmarshal(artifact.Manifest, &manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to parse artifact manifest: %w", err)
	}

	return &manifest, nil
}

// SetManifest sets the manifest for a backup artifact
func (s *ArtifactService) SetManifest(ctx context.Context, artifactID string, manifest *BackupManifest) error {
	artifact, err := s.Get(ctx, artifactID)
	if err != nil {
		return fmt.Errorf("failed to get backup artifact: %w", err)
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	artifact.Manifest = data
	return s.persistArtifact(ctx, artifact)
}

// CleanupExpired cleans up expired backup artifacts
func (s *ArtifactService) CleanupExpired(ctx context.Context, retentionDays int) (int, error) {
	artifacts, _, err := s.List(ctx, ArtifactFilter{Page: 1, PerPage: 200})
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().UTC()
	if retentionDays > 0 {
		cutoff = cutoff.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	}
	deleted := 0
	for _, artifact := range artifacts {
		expired := artifact.ExpiresAt != nil && artifact.ExpiresAt.Before(time.Now().UTC())
		olderThanRetention := retentionDays > 0 && artifact.CreatedAt.Before(cutoff)
		if artifact.IsLocked || (!expired && !olderThanRetention) {
			continue
		}
		if err := s.Delete(ctx, artifact.ID, "system"); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// ApplyRetentionPolicy applies retention policy to backup artifacts
func (s *ArtifactService) ApplyRetentionPolicy(ctx context.Context, policy RetentionPolicy) (int, error) {
	filter := ArtifactFilter{Page: 1, PerPage: 200}
	switch policy.Scope {
	case "server":
		filter.SourceServerID = policy.ServerID
	case "app":
		filter.SourceAppID = policy.AppID
	case "database":
		filter.SourceDatabaseID = policy.DatabaseID
	case "volume":
		filter.SourceVolumeID = policy.VolumeID
	case "global":
	default:
		return 0, fmt.Errorf("unsupported retention scope %q", policy.Scope)
	}
	artifacts, _, err := s.List(ctx, filter)
	if err != nil {
		return 0, err
	}
	cutoff := time.Time{}
	if policy.RetentionDays > 0 {
		cutoff = time.Now().UTC().Add(-time.Duration(policy.RetentionDays) * 24 * time.Hour)
	}
	deleted := 0
	for index, artifact := range artifacts {
		exceedsCount := policy.MaxBackups > 0 && index >= policy.MaxBackups
		exceedsAge := !cutoff.IsZero() && artifact.CreatedAt.Before(cutoff)
		if artifact.IsLocked || (!exceedsCount && !exceedsAge) {
			continue
		}
		if err := s.Delete(ctx, artifact.ID, "retention"); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// GetStatistics gets statistics for backup artifacts
func (s *ArtifactService) GetStatistics(ctx context.Context, filters ArtifactFilter) (*ArtifactStatistics, error) {
	filters.Page = 1
	filters.PerPage = 200
	artifacts, total, err := s.List(ctx, filters)
	if err != nil {
		return nil, err
	}
	stats := &ArtifactStatistics{
		TotalCount: total, ByType: make(map[string]int),
		ByStorage: make(map[string]int), ByStatus: make(map[string]int),
	}
	now := time.Now().UTC()
	for _, artifact := range artifacts {
		stats.TotalSize += artifact.FileSize
		stats.ByType[string(artifact.ArtifactType)]++
		stats.ByStorage[artifact.StorageProvider]++
		stats.ByStatus[artifact.Status]++
		if artifact.IsVerified {
			stats.VerifiedCount++
		}
		if artifact.IsLocked {
			stats.LockedCount++
		}
		if artifact.ExpiresAt != nil && artifact.ExpiresAt.Before(now) {
			stats.ExpiredCount++
		}
		created := artifact.CreatedAt
		if stats.OldestArtifact == nil || created.Before(*stats.OldestArtifact) {
			stats.OldestArtifact = &created
		}
		if stats.NewestArtifact == nil || created.After(*stats.NewestArtifact) {
			stats.NewestArtifact = &created
		}
	}
	return stats, nil
}

// deleteFromStorage deletes an artifact from storage
func (s *ArtifactService) deleteFromStorage(ctx context.Context, artifact *BackupArtifact) error {
	if artifact.StorageProvider == "beacon" {
		target, databaseBackupID, err := s.resolveBeaconArtifact(ctx, artifact)
		if err != nil {
			return err
		}
		if databaseBackupID != "" {
			return s.daemonClient.DeleteDatabaseBackup(ctx, target.NodeURL, target.NodeToken, databaseBackupID)
		}
		return s.daemonClient.DeleteBackup(ctx, target.NodeURL, target.NodeToken, target.ServerID, artifact.StoragePath)
	}
	adapter, err := s.getStorageAdapter(artifact.StorageProvider)
	if err != nil {
		return fmt.Errorf("failed to get storage adapter: %w", err)
	}

	err = adapter.Delete(ctx, artifact.StoragePath)
	if err != nil {
		return fmt.Errorf("failed to delete from storage: %w", err)
	}

	s.logger.Infof("Deleted artifact from storage: %s", artifact.StoragePath)
	return nil
}

func (s *ArtifactService) downloadFromBeacon(ctx context.Context, artifact *BackupArtifact) (io.ReadCloser, error) {
	target, databaseBackupID, err := s.resolveBeaconArtifact(ctx, artifact)
	if err != nil {
		return nil, err
	}
	if databaseBackupID != "" {
		return s.daemonClient.DownloadDatabaseBackup(ctx, target.NodeURL, target.NodeToken, databaseBackupID)
	}
	return s.daemonClient.DownloadBackup(ctx, target.NodeURL, target.NodeToken, target.ServerID, artifact.StoragePath)
}

func (s *ArtifactService) resolveBeaconArtifact(ctx context.Context, artifact *BackupArtifact) (store.ServerControlTarget, string, error) {
	if s.daemonClient == nil {
		return store.ServerControlTarget{}, "", fmt.Errorf("daemon client not available")
	}
	var serverID string
	var databaseBackupID string
	switch artifact.ArtifactType {
	case BackupTypeServer:
		if artifact.SourceServerID != nil {
			serverID = *artifact.SourceServerID
		}
	case BackupTypeApp:
		if artifact.SourceAppID == nil {
			return store.ServerControlTarget{}, "", fmt.Errorf("artifact has no source app")
		}
		app, err := s.store.GetApplication(ctx, *artifact.SourceAppID)
		if err != nil {
			return store.ServerControlTarget{}, "", err
		}
		if app.ServerID != nil {
			serverID = *app.ServerID
		}
	case BackupTypeDatabase:
		if artifact.SourceDatabaseID == nil {
			return store.ServerControlTarget{}, "", fmt.Errorf("artifact has no source database")
		}
		database, err := s.store.GetDBContainerBackupTarget(ctx, *artifact.SourceDatabaseID)
		if err != nil {
			return store.ServerControlTarget{}, "", err
		}
		serverID = database.ServerID
		databaseBackupID = strings.TrimSuffix(filepath.Base(artifact.StoragePath), ".backup.gz")
	case BackupTypeVolume:
		if artifact.JobID == nil {
			return store.ServerControlTarget{}, "", fmt.Errorf("volume artifact has no source job")
		}
		record, err := s.store.GetBackupJob(ctx, *artifact.JobID)
		if err != nil {
			return store.ServerControlTarget{}, "", err
		}
		job := backupJobFromStore(record)
		if job.ServerID != nil {
			serverID = *job.ServerID
		}
	}
	if serverID == "" {
		return store.ServerControlTarget{}, "", fmt.Errorf("artifact has no backing server")
	}
	target, err := s.store.ServerControlTarget(ctx, serverID)
	return target, databaseBackupID, err
}

// downloadToTempFile downloads an artifact to a temporary file
func (s *ArtifactService) downloadToTempFile(ctx context.Context, adapter StorageAdapter, storagePath string) (string, error) {
	// Create temp directory
	tempDir := os.TempDir()
	artifactName := filepath.Base(storagePath)
	// Sanitize the artifact name for filesystem
	safeName := strings.ReplaceAll(artifactName, "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")
	tempFile := filepath.Join(tempDir, fmt.Sprintf("backup-verify-%s", safeName))

	// Download the file
	data, err := adapter.Download(ctx, storagePath)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}

	err = os.WriteFile(tempFile, data, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	return tempFile, nil
}

// getStorageAdapter gets the storage adapter for a provider
func (s *ArtifactService) getStorageAdapter(provider string) (StorageAdapter, error) {
	adapter, ok := s.storageAdapters[provider]
	if !ok {
		if s.defaultAdapter != nil {
			s.logger.Warnf("Storage provider %s not found, using default adapter", provider)
			return s.defaultAdapter, nil
		}
		return nil, fmt.Errorf("storage provider %s not found and no default adapter", provider)
	}
	return adapter, nil
}

func backupArtifactToRecord(artifact *BackupArtifact) (store.BackupArtifactRecord, error) {
	data, err := json.Marshal(artifact)
	if err != nil {
		return store.BackupArtifactRecord{}, fmt.Errorf("encode backup artifact: %w", err)
	}
	return store.BackupArtifactRecord{
		ID: artifact.ID, JobID: artifact.JobID, ConfigurationID: artifact.ConfigurationID,
		ArtifactType: string(artifact.ArtifactType), Name: artifact.Name,
		DisplayName: artifact.DisplayName, StorageProvider: artifact.StorageProvider,
		StoragePath: artifact.StoragePath, FileSize: artifact.FileSize,
		FileHash: artifact.FileHash, Status: artifact.Status,
		IsVerified: artifact.IsVerified, IsLocked: artifact.IsLocked,
		LockReason: artifact.LockReason, Data: data, CreatedAt: artifact.CreatedAt,
		UpdatedAt: artifact.UpdatedAt, UploadedAt: artifact.UploadedAt,
	}, nil
}

func backupArtifactFromRecord(record store.BackupArtifactRecord) *BackupArtifact {
	var artifact BackupArtifact
	_ = json.Unmarshal(record.Data, &artifact)
	artifact.ID = record.ID
	artifact.JobID = record.JobID
	artifact.ConfigurationID = record.ConfigurationID
	artifact.ArtifactType = BackupType(record.ArtifactType)
	artifact.Name = record.Name
	artifact.DisplayName = record.DisplayName
	artifact.StorageProvider = record.StorageProvider
	artifact.StoragePath = record.StoragePath
	artifact.FileSize = record.FileSize
	artifact.FileHash = record.FileHash
	artifact.Status = record.Status
	artifact.IsVerified = record.IsVerified
	artifact.IsLocked = record.IsLocked
	artifact.LockReason = record.LockReason
	artifact.CreatedAt = record.CreatedAt
	artifact.UpdatedAt = record.UpdatedAt
	artifact.UploadedAt = record.UploadedAt
	return &artifact
}

func (s *ArtifactService) persistArtifact(ctx context.Context, artifact *BackupArtifact) error {
	record, err := backupArtifactToRecord(artifact)
	if err != nil {
		return err
	}
	if err := s.store.UpdateBackupArtifactRecord(ctx, &record); err != nil {
		return fmt.Errorf("persist backup artifact: %w", err)
	}
	artifact.UpdatedAt = time.Now().UTC()
	return nil
}

// calculateFileHash calculates the hash of a file
func calculateFileHash(filePath string, algorithm string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var hasher = sha256.New()
	if algorithm != "sha256" && algorithm != "" {
		// For now, only sha256 is supported
		// Could add support for other algorithms like md5, sha1, etc.
		return "", fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}

	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// CreateArtifactRequest represents a request to create a backup artifact
type CreateArtifactRequest struct {
	JobID                *string         `json:"jobId,omitempty"`
	ConfigurationID      *string         `json:"configurationId,omitempty"`
	ArtifactType         BackupType      `json:"artifactType"`
	Name                 string          `json:"name"`
	DisplayName          string          `json:"displayName,omitempty"`
	StorageProvider      string          `json:"storageProvider,omitempty"`
	StoragePath          string          `json:"storagePath"`
	StorageURL           *string         `json:"storageUrl,omitempty"`
	FileSize             int64           `json:"fileSize"`
	FileHash             *string         `json:"fileHash,omitempty"`
	HashAlgorithm        string          `json:"hashAlgorithm,omitempty"`
	SourceServerID       *string         `json:"sourceServerId,omitempty"`
	SourceAppID          *string         `json:"sourceAppId,omitempty"`
	SourceDatabaseID     *string         `json:"sourceDatabaseId,omitempty"`
	SourceVolumeID       *string         `json:"sourceVolumeId,omitempty"`
	DatabaseEngine       *DatabaseEngine `json:"databaseEngine,omitempty"`
	DatabaseName         *string         `json:"databaseName,omitempty"`
	VolumeName           *string         `json:"volumeName,omitempty"`
	VolumeMountPath      *string         `json:"volumeMountPath,omitempty"`
	AppName              *string         `json:"appName,omitempty"`
	AppVersion           *string         `json:"appVersion,omitempty"`
	IsCompressed         bool            `json:"isCompressed"`
	CompressionAlgorithm *string         `json:"compressionAlgorithm,omitempty"`
	IsEncrypted          bool            `json:"isEncrypted"`
	EncryptionAlgorithm  *string         `json:"encryptionAlgorithm,omitempty"`
	Status               string          `json:"status,omitempty"`
	IsVerified           bool            `json:"isVerified"`
	IsLocked             bool            `json:"isLocked"`
	LockReason           *string         `json:"lockReason,omitempty"`
	ExpiresAt            *time.Time      `json:"expiresAt,omitempty"`
	RetentionDays        int             `json:"retentionDays,omitempty"`
	Manifest             json.RawMessage `json:"manifest,omitempty"`
	Metadata             json.RawMessage `json:"metadata,omitempty"`
}

// ArtifactStatistics represents statistics for backup artifacts
type ArtifactStatistics struct {
	TotalCount     int            `json:"totalCount"`
	TotalSize      int64          `json:"totalSize"`
	VerifiedCount  int            `json:"verifiedCount"`
	LockedCount    int            `json:"lockedCount"`
	ExpiredCount   int            `json:"expiredCount"`
	ByType         map[string]int `json:"byType,omitempty"`
	ByStorage      map[string]int `json:"byStorage,omitempty"`
	ByStatus       map[string]int `json:"byStatus,omitempty"`
	OldestArtifact *time.Time     `json:"oldestArtifact,omitempty"`
	NewestArtifact *time.Time     `json:"newestArtifact,omitempty"`
}

// bytesReader is a helper to create a reader from bytes
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *bytesReader) Close() error {
	r.data = nil
	r.pos = 0
	return nil
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data, pos: 0}
}
