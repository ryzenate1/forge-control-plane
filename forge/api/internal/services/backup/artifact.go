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

	s.logger.Infof("Created backup artifact from result: %s (job: %s)", artifact.Name, job.ID)

	return artifact, nil
}

// Get retrieves a backup artifact by ID
func (s *ArtifactService) Get(ctx context.Context, artifactID string) (*BackupArtifact, error) {
	// TODO: Implement database retrieval
	// Placeholder implementation
	return nil, fmt.Errorf("not implemented: Get backup artifact")
}

// List retrieves backup artifacts with optional filtering
func (s *ArtifactService) List(ctx context.Context, filters ArtifactFilter) ([]*BackupArtifact, int, error) {
	// TODO: Implement database listing with filters
	// Placeholder implementation
	return []*BackupArtifact{}, 0, nil
}

// Update updates a backup artifact
func (s *ArtifactService) Update(ctx context.Context, artifactID string, updates map[string]interface{}) (*BackupArtifact, error) {
	// TODO: Implement database update
	// Placeholder implementation
	return nil, fmt.Errorf("not implemented: Update backup artifact")
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
	err = s.deleteFromStorage(ctx, artifact)
	if err != nil {
		s.logger.Warnf("Failed to delete artifact from storage: %v", err)
		// Continue with database deletion even if storage deletion fails
	}

	// TODO: Delete from database
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

	// TODO: Update in database
	artifact.IsLocked = true
	artifact.LockReason = &reason

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

	// TODO: Update in database
	artifact.IsLocked = false
	artifact.LockReason = nil

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

	// Get the storage adapter
	adapter, err := s.getStorageAdapter(artifact.StorageProvider)
	if err != nil {
		return fmt.Errorf("failed to get storage adapter: %w", err)
	}

	// Download the artifact to verify
	tempFile, err := s.downloadToTempFile(ctx, adapter, artifact.StoragePath)
	if err != nil {
		return fmt.Errorf("failed to download artifact for verification: %w", err)
	}
	defer os.Remove(tempFile)

	// Calculate hash of the downloaded file
	calculatedHash, err := calculateFileHash(tempFile, artifact.HashAlgorithm)
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %w", err)
	}

	// Compare with stored hash
	if artifact.FileHash == nil || *artifact.FileHash == "" {
		// No stored hash, just mark as verified
		s.logger.Warnf("No stored hash for artifact %s, skipping hash verification", artifact.Name)
	} else if *artifact.FileHash != calculatedHash {
		artifact.VerificationAttempts++
		// TODO: Update in database
		return fmt.Errorf("hash mismatch: expected %s, got %s", *artifact.FileHash, calculatedHash)
	}

	// Mark as verified
	artifact.IsVerified = true
	artifact.VerificationAttempts = 0
	now := time.Now()
	artifact.LastVerifiedAt = &now
	artifact.Status = "verified"

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

	// Get the storage adapter
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

	// TODO: Update in database

	return nil
}

// CleanupExpired cleans up expired backup artifacts
func (s *ArtifactService) CleanupExpired(ctx context.Context, retentionDays int) (int, error) {
	// TODO: Implement cleanup logic
	// This would:
	// 1. Find all artifacts where expires_at < now() and is_locked = false
	// 2. Delete them from storage and database
	// 3. Return the count of deleted artifacts

	s.logger.Infof("Cleaning up expired backup artifacts (retention: %d days)", retentionDays)

	return 0, nil
}

// ApplyRetentionPolicy applies retention policy to backup artifacts
func (s *ArtifactService) ApplyRetentionPolicy(ctx context.Context, policy RetentionPolicy) (int, error) {
	// TODO: Implement retention policy application
	// This would:
	// 1. Find artifacts matching the policy scope
	// 2. Sort by creation date
	// 3. Keep the most recent N artifacts (based on policy)
	// 4. Delete the rest (if not locked)

	s.logger.Infof("Applying retention policy: %s", policy.Name)

	return 0, nil
}

// GetStatistics gets statistics for backup artifacts
func (s *ArtifactService) GetStatistics(ctx context.Context, filters ArtifactFilter) (*ArtifactStatistics, error) {
	// TODO: Implement statistics calculation
	return &ArtifactStatistics{}, nil
}

// deleteFromStorage deletes an artifact from storage
func (s *ArtifactService) deleteFromStorage(ctx context.Context, artifact *BackupArtifact) error {
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
