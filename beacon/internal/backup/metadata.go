package backup

import "time"

type Backup struct {
	ID          string
	ServerID    string
	StartedAt   time.Time
	CompletedAt time.Time
	Status      BackupStatus
	SizeBytes   int64
	Files       int
	Duration    time.Duration
	Adapter     string
	Path        string
	Error       string
}

type BackupStatus string

const (
	BackupStatusPending   BackupStatus = "pending"
	BackupStatusRunning   BackupStatus = "running"
	BackupStatusCompleted BackupStatus = "completed"
	BackupStatusFailed    BackupStatus = "failed"
)
