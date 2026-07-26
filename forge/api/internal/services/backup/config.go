package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

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
	store      *store.Store
	logger     Logger
	scheduler  Scheduler
	jobService *JobService
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

	if err := validateBackupTargets(req.BackupType, req.ServerID, req.AppID, req.DatabaseID, req.VolumeID); err != nil {
		return nil, err
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

	config := &BackupConfig{
		ID:                 uuid.NewString(),
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

	if err := config.Validate(); err != nil {
		return nil, err
	}
	record, err := backupConfigToRecord(config)
	if err != nil {
		return nil, err
	}
	if err := s.store.CreateBackupConfiguration(ctx, &record); err != nil {
		return nil, fmt.Errorf("persist backup configuration: %w", err)
	}
	config.CreatedAt = record.CreatedAt
	config.UpdatedAt = record.UpdatedAt

	s.logger.Infof("Created backup configuration: %s (type: %s, target: %s)", config.Name, config.BackupType, config.getTargetDescription())

	return config, nil
}

// Get retrieves a backup configuration by ID
func (s *ConfigService) Get(ctx context.Context, configID string) (*BackupConfig, error) {
	record, err := s.store.GetBackupConfiguration(ctx, configID)
	if err != nil {
		return nil, fmt.Errorf("get backup configuration: %w", err)
	}
	return backupConfigFromRecord(record), nil
}

// List retrieves all backup configurations with optional filtering
func (s *ConfigService) List(ctx context.Context, filters BackupConfigFilter) ([]*BackupConfig, int, error) {
	records, err := s.store.ListBackupConfigurations(ctx)
	if err != nil {
		return nil, 0, err
	}
	configs := make([]*BackupConfig, 0, len(records))
	for _, record := range records {
		config := backupConfigFromRecord(record)
		if filters.Search != nil {
			query := strings.ToLower(strings.TrimSpace(*filters.Search))
			if query != "" && !strings.Contains(strings.ToLower(config.Name+" "+config.Description), query) {
				continue
			}
		}
		if filters.BackupType != nil && config.BackupType != *filters.BackupType {
			continue
		}
		if filters.ServerID != nil && !sameStringPointer(config.ServerID, filters.ServerID) {
			continue
		}
		if filters.AppID != nil && !sameStringPointer(config.AppID, filters.AppID) {
			continue
		}
		if filters.DatabaseID != nil && !sameStringPointer(config.DatabaseID, filters.DatabaseID) {
			continue
		}
		if filters.VolumeID != nil && !sameStringPointer(config.VolumeID, filters.VolumeID) {
			continue
		}
		if filters.Enabled != nil && config.Enabled != *filters.Enabled {
			continue
		}
		if filters.Scheduled != nil && config.IsScheduled != *filters.Scheduled {
			continue
		}
		configs = append(configs, config)
	}
	total := len(configs)
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
	return configs[start:end], total, nil
}

// Update updates an existing backup configuration
func (s *ConfigService) Update(ctx context.Context, configID string, req UpdateBackupConfigRequest, userID string) (*BackupConfig, error) {
	config, err := s.Get(ctx, configID)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		config.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		config.Description = *req.Description
	}
	if req.IsScheduled != nil {
		config.IsScheduled = *req.IsScheduled
	}
	if req.CronExpression != nil {
		config.CronExpression = req.CronExpression
	}
	if req.StorageProvider != nil {
		config.StorageProvider = *req.StorageProvider
	}
	if len(req.StorageConfig) > 0 {
		config.StorageConfig = req.StorageConfig
	}
	if req.MaxBackups != nil {
		config.MaxBackups = *req.MaxBackups
	}
	if req.RetentionDays != nil {
		config.RetentionDays = *req.RetentionDays
	}
	if req.CompressionEnabled != nil {
		config.CompressionEnabled = *req.CompressionEnabled
	}
	if req.EncryptionEnabled != nil {
		config.EncryptionEnabled = *req.EncryptionEnabled
	}
	if req.EncryptionKeyID != nil {
		config.EncryptionKeyID = req.EncryptionKeyID
	}
	if req.Enabled != nil {
		config.Enabled = *req.Enabled
	}
	if config.IsScheduled {
		if config.CronExpression == nil || strings.TrimSpace(*config.CronExpression) == "" {
			return nil, errors.New("cron expression is required for scheduled backups")
		}
		nextRun, err := s.calculateNextCronRun(*config.CronExpression)
		if err != nil {
			return nil, err
		}
		config.NextRunAt = &nextRun
	} else {
		config.NextRunAt = nil
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	record, err := backupConfigToRecord(config)
	if err != nil {
		return nil, err
	}
	if err := s.store.UpdateBackupConfiguration(ctx, &record); err != nil {
		return nil, fmt.Errorf("persist backup configuration update: %w", err)
	}
	config.UpdatedAt = record.UpdatedAt
	s.logger.Infof("Updated backup configuration %s by user %s", configID, userID)
	return config, nil
}

// Delete deletes a backup configuration
func (s *ConfigService) Delete(ctx context.Context, configID string, userID string) error {
	if err := s.store.DeleteBackupConfiguration(ctx, configID); err != nil {
		return fmt.Errorf("delete backup configuration: %w", err)
	}
	s.logger.Infof("Deleted backup configuration %s by user %s", configID, userID)
	return nil
}

// Enable enables a backup configuration
func (s *ConfigService) Enable(ctx context.Context, configID string, userID string) error {
	enabled := true
	_, err := s.Update(ctx, configID, UpdateBackupConfigRequest{Enabled: &enabled}, userID)
	return err
}

// Disable disables a backup configuration
func (s *ConfigService) Disable(ctx context.Context, configID string, userID string) error {
	enabled := false
	_, err := s.Update(ctx, configID, UpdateBackupConfigRequest{Enabled: &enabled}, userID)
	return err
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
	if s.jobService == nil {
		return nil, fmt.Errorf("backup job service is unavailable")
	}
	job, err := s.jobService.CreateFromConfig(ctx, config, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup job: %w", err)
	}

	// Execute the job
	err = s.jobService.Execute(ctx, job.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to execute backup job: %w", err)
	}
	return s.jobService.Get(ctx, job.ID)
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
	schedule, err := cron.ParseStandard(strings.TrimSpace(cronExpr))
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(time.Now().UTC()), nil
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

	if err := validateBackupTargets(c.BackupType, c.ServerID, c.AppID, c.DatabaseID, c.VolumeID); err != nil {
		return err
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
		if _, err := cron.ParseStandard(strings.TrimSpace(*c.CronExpression)); err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
	}

	return nil
}

func validateBackupTargets(backupType BackupType, serverID, appID, databaseID, volumeID *string) error {
	has := func(value *string) bool { return value != nil && strings.TrimSpace(*value) != "" }
	switch backupType {
	case BackupTypeServer:
		if !has(serverID) || has(appID) || has(databaseID) || has(volumeID) {
			return fmt.Errorf("server backup requires only serverId")
		}
	case BackupTypeApp:
		if !has(appID) || has(serverID) || has(databaseID) || has(volumeID) {
			return fmt.Errorf("app backup requires only appId")
		}
	case BackupTypeDatabase:
		if !has(databaseID) || has(serverID) || has(appID) || has(volumeID) {
			return fmt.Errorf("database backup requires only databaseId")
		}
	case BackupTypeVolume:
		if !has(serverID) || !has(volumeID) || has(appID) || has(databaseID) {
			return fmt.Errorf("volume backup requires serverId and volumeId")
		}
	default:
		return fmt.Errorf("invalid backup type: %s", backupType)
	}
	return nil
}

func sameStringPointer(left, right *string) bool {
	return left != nil && right != nil && *left == *right
}

func backupConfigToRecord(config *BackupConfig) (store.BackupConfigurationRecord, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return store.BackupConfigurationRecord{}, fmt.Errorf("encode backup configuration: %w", err)
	}
	return store.BackupConfigurationRecord{
		ID: config.ID, Name: config.Name, Description: config.Description,
		ServerID: config.ServerID, AppID: config.AppID, DatabaseID: config.DatabaseID,
		VolumeID: config.VolumeID, BackupType: string(config.BackupType),
		IsScheduled: config.IsScheduled, CronExpression: config.CronExpression,
		NextRunAt: config.NextRunAt, LastRunAt: config.LastRunAt,
		StorageProvider: config.StorageProvider, StorageConfig: config.StorageConfig,
		MaxBackups: config.MaxBackups, RetentionDays: config.RetentionDays,
		CompressionEnabled: config.CompressionEnabled, EncryptionEnabled: config.EncryptionEnabled,
		EncryptionKeyID: config.EncryptionKeyID, Enabled: config.Enabled,
		LastStatus: config.LastStatus, LastError: config.LastError, Data: data,
		CreatedAt: config.CreatedAt, UpdatedAt: config.UpdatedAt,
	}, nil
}

func backupConfigFromRecord(record store.BackupConfigurationRecord) *BackupConfig {
	var config BackupConfig
	_ = json.Unmarshal(record.Data, &config)
	config.ID = record.ID
	config.Name = record.Name
	config.Description = record.Description
	config.ServerID = record.ServerID
	config.AppID = record.AppID
	config.DatabaseID = record.DatabaseID
	config.VolumeID = record.VolumeID
	config.BackupType = BackupType(record.BackupType)
	config.IsScheduled = record.IsScheduled
	config.CronExpression = record.CronExpression
	config.NextRunAt = record.NextRunAt
	config.LastRunAt = record.LastRunAt
	config.StorageProvider = record.StorageProvider
	config.StorageConfig = record.StorageConfig
	config.MaxBackups = record.MaxBackups
	config.RetentionDays = record.RetentionDays
	config.CompressionEnabled = record.CompressionEnabled
	config.EncryptionEnabled = record.EncryptionEnabled
	config.EncryptionKeyID = record.EncryptionKeyID
	config.Enabled = record.Enabled
	config.LastStatus = record.LastStatus
	config.LastError = record.LastError
	config.CreatedAt = record.CreatedAt
	config.UpdatedAt = record.UpdatedAt
	return &config
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
