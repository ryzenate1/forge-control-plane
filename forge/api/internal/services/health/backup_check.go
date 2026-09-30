package health

import (
	"context"
	"os"
	"time"
)

type BackupCheck struct {
	label     string
	backupDir string
	timeout   time.Duration
}

func NewBackupCheck(backupDir string) *BackupCheck {
	if backupDir == "" {
		backupDir = "/var/backups"
	}
	return &BackupCheck{
		label:     "Backup System",
		backupDir: backupDir,
		timeout:   3 * time.Second,
	}
}

func (c *BackupCheck) Name() string  { return "backup" }
func (c *BackupCheck) Label() string { return c.label }

func (c *BackupCheck) Run(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:   c.Name(),
		Label:  c.Label(),
		Status: StatusOK,
	}

	info, err := os.Stat(c.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			result.Status = StatusWarning
			result.Message = "Backup directory does not exist"
		} else {
			result.Status = StatusFailed
			result.Message = "Backup directory stat failed: " + err.Error()
		}
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	if !info.IsDir() {
		result.Status = StatusFailed
		result.Message = "Backup path is not a directory"
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	entries, err := os.ReadDir(c.backupDir)
	if err != nil {
		result.Status = StatusFailed
		result.Message = "Cannot read backup directory: " + err.Error()
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	var totalSize int64
	var fileCount int
	var recentCount int
	now := time.Now()
	for _, entry := range entries {
		fi, err := entry.Info()
		if err != nil {
			continue
		}
		totalSize += fi.Size()
		fileCount++
		if now.Sub(fi.ModTime()) < 24*time.Hour {
			recentCount++
		}
	}

	_ = totalSize
	_ = recentCount

	if fileCount == 0 {
		result.Status = StatusWarning
		result.Message = "Backup directory is empty"
	} else {
		result.Message = "Backup system accessible"
	}

	result.LatencyMs = time.Since(start).Milliseconds()
	return result
}

var _ Check = (*BackupCheck)(nil)
