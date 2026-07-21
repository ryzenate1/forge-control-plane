package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Logger is the interface for logging
type Logger interface {
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Panic(args ...interface{})
	Panicf(format string, args ...interface{})
}

// Scheduler is the interface for scheduling backup jobs
type Scheduler interface {
	// ScheduleBackupJob schedules a backup job to run on a cron schedule
	ScheduleBackupJob(ctx context.Context, configID string, cronExpr string) error

	// UnscheduleBackupJob removes a scheduled backup job
	UnscheduleBackupJob(ctx context.Context, configID string) error

	// ListScheduledJobs lists all scheduled backup jobs
	ListScheduledJobs(ctx context.Context) ([]ScheduledJob, error)

	// GetNextRunTime gets the next run time for a cron expression
	GetNextRunTime(cronExpr string) (time.Time, error)
}

// ScheduledJob represents a scheduled job
type ScheduledJob struct {
	ID         string     `json:"id"`
	ConfigID   string     `json:"configId"`
	CronExpr   string     `json:"cronExpr"`
	NextRunAt  time.Time  `json:"nextRunAt"`
	LastRunAt  *time.Time `json:"lastRunAt,omitempty"`
	LastStatus string     `json:"lastStatus,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// BeaconClient is the interface for communicating with Beacon nodes
type BeaconClient interface {
	// ExecuteBackup executes a backup on a node
	ExecuteBackup(ctx context.Context, nodeID string, backupType BackupType, targetID string, name string, storage StorageAdapter) (string, error)

	// ExecuteDatabaseBackup executes a database backup on a node
	ExecuteDatabaseBackup(ctx context.Context, nodeID string, engine DatabaseEngine, databaseID string, name string, storage StorageAdapter) (string, error)

	// ExecuteRestore executes a restore on a node
	ExecuteRestore(ctx context.Context, nodeID string, restoreType BackupType, targetID string, backupFile string, restorePath string, options RestoreOptions) (string, error)

	// ExecuteDatabaseRestore executes a database restore on a node
	ExecuteDatabaseRestore(ctx context.Context, nodeID string, engine DatabaseEngine, databaseID string, backupFile string, options RestoreOptions) (string, error)

	// GetBackupResult gets the result of a backup operation
	GetBackupResult(ctx context.Context, taskID string) (*BackupResult, error)

	// GetRestoreResult gets the result of a restore operation
	GetRestoreResult(ctx context.Context, taskID string) (*RestoreResult, error)

	// GetTaskStatus gets the status of a task
	GetTaskStatus(ctx context.Context, taskID string) (string, error)

	// CancelTask cancels a running task
	CancelTask(ctx context.Context, taskID string) error
}

// RestoreResult represents the result of a restore operation from beacon
type RestoreResult struct {
	Success       bool            `json:"success"`
	Message       string          `json:"message,omitempty"`
	Error         *string         `json:"error,omitempty"`
	BytesRestored int64           `json:"bytesRestored"`
	FilesRestored int             `json:"filesRestored"`
	Duration      float64         `json:"duration"`
	Details       json.RawMessage `json:"details,omitempty"`
}

// StorageAdapter is the interface for storage providers
// This extends the existing StorageAdapter interface with additional methods
type StorageAdapter interface {
	// Name returns the name of the storage adapter
	Name() string

	// Upload uploads data to the storage
	Upload(ctx context.Context, path string, data []byte) error

	// Download downloads data from the storage
	Download(ctx context.Context, path string) ([]byte, error)

	// Delete deletes data from the storage
	Delete(ctx context.Context, path string) error

	// List lists files in the storage with the given prefix
	List(ctx context.Context, prefix string) ([]string, error)

	// Exists checks if a file exists in the storage
	Exists(ctx context.Context, path string) (bool, error)

	// UploadStream uploads data from a stream
	UploadStream(ctx context.Context, path string, reader io.Reader, size int64) error

	// DownloadStream downloads data to a stream
	DownloadStream(ctx context.Context, path string) (io.Reader, error)

	// GetFileInfo gets information about a file
	GetFileInfo(ctx context.Context, path string) (FileInfo, error)
}

// FileInfo represents information about a file in storage
type FileInfo struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	ETag     string    `json:"etag,omitempty"`
	IsDir    bool      `json:"isDir"`
}

// NoOpLogger is a no-op logger implementation
type NoOpLogger struct{}

func (l *NoOpLogger) Debug(args ...interface{})                 {}
func (l *NoOpLogger) Debugf(format string, args ...interface{}) {}
func (l *NoOpLogger) Info(args ...interface{})                  {}
func (l *NoOpLogger) Infof(format string, args ...interface{})  {}
func (l *NoOpLogger) Warn(args ...interface{})                  {}
func (l *NoOpLogger) Warnf(format string, args ...interface{})  {}
func (l *NoOpLogger) Error(args ...interface{})                 {}
func (l *NoOpLogger) Errorf(format string, args ...interface{}) {}
func (l *NoOpLogger) Fatal(args ...interface{})                 {}
func (l *NoOpLogger) Fatalf(format string, args ...interface{}) {}
func (l *NoOpLogger) Panic(args ...interface{})                 {}
func (l *NoOpLogger) Panicf(format string, args ...interface{}) {}

// NewNoOpLogger creates a new no-op logger
func NewNoOpLogger() Logger {
	return &NoOpLogger{}
}

// NoOpScheduler is a no-op scheduler implementation
type NoOpScheduler struct{}

func (s *NoOpScheduler) ScheduleBackupJob(ctx context.Context, configID string, cronExpr string) error {
	return nil
}

func (s *NoOpScheduler) UnscheduleBackupJob(ctx context.Context, configID string) error {
	return nil
}

func (s *NoOpScheduler) ListScheduledJobs(ctx context.Context) ([]ScheduledJob, error) {
	return []ScheduledJob{}, nil
}

func (s *NoOpScheduler) GetNextRunTime(cronExpr string) (time.Time, error) {
	return time.Now().Add(24 * time.Hour), nil
}

// NewNoOpScheduler creates a new no-op scheduler
func NewNoOpScheduler() Scheduler {
	return &NoOpScheduler{}
}

// NoOpBeaconClient is a no-op beacon client implementation
type NoOpBeaconClient struct{}

func (c *NoOpBeaconClient) ExecuteBackup(ctx context.Context, nodeID string, backupType BackupType, targetID string, name string, storage StorageAdapter) (string, error) {
	return "", fmt.Errorf("beacon client not available")
}

func (c *NoOpBeaconClient) ExecuteDatabaseBackup(ctx context.Context, nodeID string, engine DatabaseEngine, databaseID string, name string, storage StorageAdapter) (string, error) {
	return "", fmt.Errorf("beacon client not available")
}

func (c *NoOpBeaconClient) ExecuteRestore(ctx context.Context, nodeID string, restoreType BackupType, targetID string, backupFile string, restorePath string, options RestoreOptions) (string, error) {
	return "", fmt.Errorf("beacon client not available")
}

func (c *NoOpBeaconClient) ExecuteDatabaseRestore(ctx context.Context, nodeID string, engine DatabaseEngine, databaseID string, backupFile string, options RestoreOptions) (string, error) {
	return "", fmt.Errorf("beacon client not available")
}

func (c *NoOpBeaconClient) GetBackupResult(ctx context.Context, taskID string) (*BackupResult, error) {
	return nil, fmt.Errorf("beacon client not available")
}

func (c *NoOpBeaconClient) GetRestoreResult(ctx context.Context, taskID string) (*RestoreResult, error) {
	return nil, fmt.Errorf("beacon client not available")
}

func (c *NoOpBeaconClient) GetTaskStatus(ctx context.Context, taskID string) (string, error) {
	return "unknown", fmt.Errorf("beacon client not available")
}

func (c *NoOpBeaconClient) CancelTask(ctx context.Context, taskID string) error {
	return fmt.Errorf("beacon client not available")
}

// NewNoOpBeaconClient creates a new no-op beacon client
func NewNoOpBeaconClient() BeaconClient {
	return &NoOpBeaconClient{}
}
