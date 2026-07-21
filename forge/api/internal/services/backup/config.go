package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gamepanel/forge/internal/store"
)

// BackupConfig represents a backup configuration template
type BackupConfig struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Description        string          `json:"description,omitempty"`
	ServerID           *string         `json:"serverId,omitempty"`
	AppID              *string         `json:"appId,omitempty"`
	DatabaseID         *string         `json:"databaseId,omitempty"`
	VolumeID           *string         `json:"volumeId,omitempty"`
	BackupType         BackupType      `json:"backupType"`
	IsScheduled        bool            `json:"isScheduled"`
	CronExpression     *string         `json:"cronExpression,omitempty"`
	NextRunAt          *time.Time      `json:"nextRunAt,omitempty"`
	LastRunAt          *time.Time      `json:"lastRunAt,omitempty"`
	StorageProvider    string          `json:"storageProvider"`
	StorageConfig      json.RawMessage `json:"storageConfig,omitempty"`
	MaxBackups         int             `json:"maxBackups"`
	RetentionDays      int             `json:"retentionDays"`
	CompressionEnabled bool            `json:"compressionEnabled"`
	EncryptionEnabled  bool            `json:"encryptionEnabled"`
	EncryptionKeyID    *string         `json:"encryptionKeyId,omitempty"`
	Enabled            bool            `json:"enabled"`
	LastStatus         *string         `json:"lastStatus,omitempty"`
	LastError          *string         `json:"lastError,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

// StorageConfig represents configuration for storage providers
type StorageConfig struct {
	Local *LocalStorageConfig `json:"local,omitempty"`
	S3    *S3StorageConfig    `json:"s3,omitempty"`
	Azure *AzureStorageConfig `json:"azure,omitempty"`
	GCS   *GCSStorageConfig   `json:"gcs,omitempty"`
	MinIO *MinIOStorageConfig `json:"minio,omitempty"`
}

// LocalStorageConfig represents configuration for local storage
type LocalStorageConfig struct {
	BasePath string `json:"basePath"`
}

// S3StorageConfig represents configuration for S3-compatible storage
type S3StorageConfig struct {
	Region          string `json:"region"`
	Endpoint        string `json:"endpoint,omitempty"`
	Bucket          string `json:"bucket"`
	Prefix          string `json:"prefix,omitempty"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	UsePathStyle    bool   `json:"usePathStyle,omitempty"`
}

// AzureStorageConfig represents configuration for Azure Blob Storage
type AzureStorageConfig struct {
	ConnectionString string `json:"connectionString"`
	Container        string `json:"container"`
	Prefix           string `json:"prefix,omitempty"`
}

// GCSStorageConfig represents configuration for Google Cloud Storage
type GCSStorageConfig struct {
	ProjectID      string `json:"projectId"`
	Bucket         string `json:"bucket"`
	Prefix         string `json:"prefix,omitempty"`
	ServiceAccount string `json:"serviceAccount"`
}

// MinIOStorageConfig represents configuration for MinIO storage
type MinIOStorageConfig struct {
	Endpoint        string `json:"endpoint"`
	Bucket          string `json:"bucket"`
	Prefix          string `json:"prefix,omitempty"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	UseSSL          bool   `json:"useSsl,omitempty"`
}

// CreateBackupConfigRequest represents a request to create a backup configuration
type CreateBackupConfigRequest struct {
	Name               string          `json:"name"`
	Description        string          `json:"description,omitempty"`
	ServerID           *string         `json:"serverId,omitempty"`
	AppID              *string         `json:"appId,omitempty"`
	DatabaseID         *string         `json:"databaseId,omitempty"`
	VolumeID           *string         `json:"volumeId,omitempty"`
	BackupType         BackupType      `json:"backupType"`
	IsScheduled        bool            `json:"isScheduled,omitempty"`
	CronExpression     *string         `json:"cronExpression,omitempty"`
	StorageProvider    string          `json:"storageProvider,omitempty"`
	StorageConfig      json.RawMessage `json:"storageConfig,omitempty"`
	MaxBackups         int             `json:"maxBackups,omitempty"`
	RetentionDays      int             `json:"retentionDays,omitempty"`
	CompressionEnabled bool            `json:"compressionEnabled,omitempty"`
	EncryptionEnabled  bool            `json:"encryptionEnabled,omitempty"`
	EncryptionKeyID    *string         `json:"encryptionKeyId,omitempty"`
	Enabled            bool            `json:"enabled,omitempty"`
}

// UpdateBackupConfigRequest represents a request to update a backup configuration
type UpdateBackupConfigRequest struct {
	Name               *string         `json:"name,omitempty"`
	Description        *string         `json:"description,omitempty"`
	IsScheduled        *bool           `json:"isScheduled,omitempty"`
	CronExpression     *string         `json:"cronExpression,omitempty"`
	StorageProvider    *string         `json:"storageProvider,omitempty"`
	StorageConfig      json.RawMessage `json:"storageConfig,omitempty"`
	MaxBackups         *int            `json:"maxBackups,omitempty"`
	RetentionDays      *int            `json:"retentionDays,omitempty"`
	CompressionEnabled *bool           `json:"compressionEnabled,omitempty"`
	EncryptionEnabled  *bool           `json:"encryptionEnabled,omitempty"`
	EncryptionKeyID    *string         `json:"encryptionKeyId,omitempty"`
	Enabled            *bool           `json:"enabled,omitempty"`
}

// ConfigService handles backup configuration management
type ConfigService struct {
	store     *store.Store
	logger    Logger
	scheduler Scheduler
}

// NewConfigService creates a new ConfigService
func NewConfigService(store *store.Store, logger Logger, scheduler Scheduler) *ConfigService {
	return &ConfigService{
		store:     store,
		logger:    logger,
		scheduler: scheduler,
	}
}

// Create creates a new backup configuration
func (s *ConfigService) Create(ctx context.Context, req CreateBackupConfigRequest, userID string) (*BackupConfig, error) {
	// Validate the request
	if req.Name == "" {
		return nil, fmt.Errorf("backup configuration name is required")
	}

	// Validate backup type
	if req.BackupType != BackupTypeApp && req.BackupType != BackupTypeVolume &&
		req.BackupType != BackupTypeDatabase && req.BackupType != BackupTypeServer {
		return nil, fmt.Errorf("invalid backup type: %s", req.BackupType)
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
	if req.StorageProvider == "" {
		req.StorageProvider = "local"
	}
	if req.MaxBackups == 0 {
		req.MaxBackups = 10
	}
	if req.RetentionDays == 0 {
		req.RetentionDays = 30
	}
	if req.CompressionEnabled {
		// Default to true if not specified
		req.CompressionEnabled = true
	}
	if !req.IsScheduled {
		// Default to manual if not specified
		req.IsScheduled = false
	}
	if req.Enabled {
		// Default to true if not specified
		req.Enabled = true
	}

	// Generate ID
	configID := uuid.NewString()

	// Create the configuration in the database
	// Note: This uses the existing store methods or direct SQL
	// For now, we'll use a placeholder since the store doesn't have these methods yet

	config := &BackupConfig{
		ID:                 configID,
		Name:               req.Name,
		Description:        req.Description,
		ServerID:           req.ServerID,
		AppID:              req.AppID,
		DatabaseID:         req.DatabaseID,
		VolumeID:           req.VolumeID,
		BackupType:         req.BackupType,
		IsScheduled:        req.IsScheduled,
		CronExpression:     req.CronExpression,
		StorageProvider:    req.StorageProvider,
		StorageConfig:      req.StorageConfig,
		MaxBackups:         req.MaxBackups,
		RetentionDays:      req.RetentionDays,
		CompressionEnabled: req.CompressionEnabled,
		EncryptionEnabled:  req.EncryptionEnabled,
		EncryptionKeyID:    req.EncryptionKeyID,
		Enabled:            req.Enabled,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	// If scheduled, calculate next run time
	if req.IsScheduled && req.CronExpression != nil {
		nextRun, err := s.calculateNextCronRun(*req.CronExpression)
		if err != nil {
			return nil, fmt.Errorf("invalid cron expression: %w", err)
		}
		config.NextRunAt = &nextRun
	}

	// TODO: Implement actual database insertion
	// This would use s.store or direct database access
	// For now, return the config as if it was created

	s.logger.Infof("Created backup configuration: %s (type: %s, target: %s)", config.Name, config.BackupType, config.getTargetDescription())

	return config, nil
}

// Get retrieves a backup configuration by ID
func (s *ConfigService) Get(ctx context.Context, configID string) (*BackupConfig, error) {
	// TODO: Implement database retrieval
	// Placeholder implementation
	return nil, fmt.Errorf("not implemented: Get backup configuration")
}

// List retrieves all backup configurations with optional filtering
func (s *ConfigService) List(ctx context.Context, filters BackupConfigFilter) ([]*BackupConfig, int, error) {
	// TODO: Implement database listing with filters
	// Placeholder implementation
	return []*BackupConfig{}, 0, nil
}

// Update updates an existing backup configuration
func (s *ConfigService) Update(ctx context.Context, configID string, req UpdateBackupConfigRequest, userID string) (*BackupConfig, error) {
	// TODO: Implement database update
	// Placeholder implementation
	return nil, fmt.Errorf("not implemented: Update backup configuration")
}

// Delete deletes a backup configuration
func (s *ConfigService) Delete(ctx context.Context, configID string, userID string) error {
	// TODO: Implement database deletion
	// Placeholder implementation
	return fmt.Errorf("not implemented: Delete backup configuration")
}

// Enable enables a backup configuration
func (s *ConfigService) Enable(ctx context.Context, configID string, userID string) error {
	// TODO: Implement enable
	return fmt.Errorf("not implemented: Enable backup configuration")
}

// Disable disables a backup configuration
func (s *ConfigService) Disable(ctx context.Context, configID string, userID string) error {
	// TODO: Implement disable
	return fmt.Errorf("not implemented: Disable backup configuration")
}

// Execute executes a backup configuration manually
func (s *ConfigService) Execute(ctx context.Context, configID string, userID string) (*BackupJob, error) {
	// Get the configuration
	config, err := s.Get(ctx, configID)
	if err != nil {
		return nil, fmt.Errorf("failed to get backup configuration: %w", err)
	}

	if !config.Enabled {
		return nil, fmt.Errorf("backup configuration is disabled")
	}

	// Create a backup job
	jobService := NewJobService(s.store, s.logger)
	job, err := jobService.CreateFromConfig(ctx, config, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup job: %w", err)
	}

	// Execute the job
	err = jobService.Execute(ctx, job.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to execute backup job: %w", err)
	}

	return job, nil
}

// BackupConfigFilter represents filters for listing backup configurations
type BackupConfigFilter struct {
	Search     *string
	BackupType *BackupType
	ServerID   *string
	AppID      *string
	DatabaseID *string
	VolumeID   *string
	Enabled    *bool
	Scheduled  *bool
	Page       int
	PerPage    int
}

// calculateNextCronRun calculates the next run time for a cron expression
func (s *ConfigService) calculateNextCronRun(cronExpr string) (time.Time, error) {
	// TODO: Implement cron parsing
	// For now, return a time 1 hour from now as a placeholder
	return time.Now().Add(1 * time.Hour), nil
}

// getTargetDescription returns a description of the backup target
func (c *BackupConfig) getTargetDescription() string {
	if c.ServerID != nil && *c.ServerID != "" {
		return fmt.Sprintf("server:%s", *c.ServerID)
	}
	if c.AppID != nil && *c.AppID != "" {
		return fmt.Sprintf("app:%s", *c.AppID)
	}
	if c.DatabaseID != nil && *c.DatabaseID != "" {
		return fmt.Sprintf("database:%s", *c.DatabaseID)
	}
	if c.VolumeID != nil && *c.VolumeID != "" {
		return fmt.Sprintf("volume:%s", *c.VolumeID)
	}
	return "unknown"
}

// Validate validates the backup configuration
func (c *BackupConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}

	if c.BackupType != BackupTypeApp && c.BackupType != BackupTypeVolume &&
		c.BackupType != BackupTypeDatabase && c.BackupType != BackupTypeServer {
		return fmt.Errorf("invalid backup type: %s", c.BackupType)
	}

	// Check that exactly one target is specified
	targetCount := 0
	if c.ServerID != nil && *c.ServerID != "" {
		targetCount++
	}
	if c.AppID != nil && *c.AppID != "" {
		targetCount++
	}
	if c.DatabaseID != nil && *c.DatabaseID != "" {
		targetCount++
	}
	if c.VolumeID != nil && *c.VolumeID != "" {
		targetCount++
	}

	if targetCount != 1 {
		return fmt.Errorf("exactly one target must be specified")
	}

	if c.StorageProvider == "" {
		return fmt.Errorf("storage provider is required")
	}

	if c.MaxBackups <= 0 {
		return fmt.Errorf("max backups must be greater than 0")
	}

	if c.RetentionDays <= 0 {
		return fmt.Errorf("retention days must be greater than 0")
	}

	if c.IsScheduled && c.CronExpression != nil && *c.CronExpression != "" {
		// TODO: Validate cron expression
	}

	return nil
}

// GetStorageConfig parses the storage configuration
func (c *BackupConfig) GetStorageConfig() (*StorageConfig, error) {
	if len(c.StorageConfig) == 0 {
		return nil, nil
	}

	var config StorageConfig
	err := json.Unmarshal(c.StorageConfig, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse storage config: %w", err)
	}

	return &config, nil
}

// SetStorageConfig sets the storage configuration
func (c *BackupConfig) SetStorageConfig(config *StorageConfig) error {
	if config == nil {
		c.StorageConfig = nil
		return nil
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal storage config: %w", err)
	}

	c.StorageConfig = data
	return nil
}
