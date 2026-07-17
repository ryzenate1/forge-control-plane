package cron

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CleanupCron struct {
	dataDir string
}

func NewCleanupCron(dataDir string) *CleanupCron {
	return &CleanupCron{dataDir: dataDir}
}

func (cc *CleanupCron) Run(ctx context.Context) error {
	if cc.dataDir == "" {
		return nil
	}
	if err := cc.removeOldTransferArchives(); err != nil {
		return err
	}
	if err := cc.removeStaleUploadSessions(); err != nil {
		return err
	}
	return cc.removeTempFiles()
}

func (cc *CleanupCron) removeOldTransferArchives() error {
	tmpDir := os.TempDir()
	cutoff := time.Now().Add(-24 * time.Hour)
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), "transfer-") || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(tmpDir, entry.Name()))
		}
	}
	return nil
}

func (cc *CleanupCron) removeStaleUploadSessions() error {
	uploadsDir := filepath.Join(cc.dataDir, ".uploads")
	cutoff := time.Now().Add(-24 * time.Hour)
	return cc.cleanDir(uploadsDir, cutoff)
}

func (cc *CleanupCron) removeTempFiles() error {
	tmpDir := filepath.Join(cc.dataDir, ".tmp")
	cutoff := time.Now().Add(-24 * time.Hour)
	return cc.cleanDir(tmpDir, cutoff)
}

func (cc *CleanupCron) cleanDir(dir string, cutoff time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				os.RemoveAll(path)
			} else {
				os.Remove(path)
			}
		}
	}
	return nil
}
