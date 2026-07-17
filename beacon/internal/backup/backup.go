package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"gamepanel/beacon/internal/metrics"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AdapterType defines the backup storage backend.
type AdapterType string

const (
	LocalAdapter AdapterType = "local"
	S3Adapter    AdapterType = "s3"
)

var (
	ErrInvalidNamespace = errors.New("invalid backup namespace")
	ErrInvalidName      = errors.New("invalid backup name")
	ErrChecksumMismatch = errors.New("backup checksum mismatch")
)

// BackupInfo contains metadata about a completed backup.
type BackupInfo struct {
	UUID        string      `json:"uuid"`
	Name        string      `json:"name"`
	Checksum    string      `json:"checksum"`
	Size        int64       `json:"size"`
	Status      string      `json:"status"`
	Created     time.Time   `json:"created"`
	CompletedAt time.Time   `json:"completedAt"`
	Adapter     AdapterType `json:"adapter"`
	RemotePath  string      `json:"remotePath,omitempty"`
}

// BackupInterface stores backups by a validated server namespace. The
// backupDir argument is retained for API compatibility, but is a namespace
// (normally the server UUID), never a filesystem path supplied by a caller.
type BackupInterface interface {
	Create(ctx context.Context, serverRoot, backupDir, name string, ignored []string) (*BackupInfo, error)
	List(backupDir string) ([]BackupInfo, error)
	Get(backupDir, name string) (*BackupInfo, error)
	Delete(backupDir, name string) error
	Restore(ctx context.Context, backupDir, name, serverRoot string, truncate bool) error
	Download(backupDir, name string) (io.ReadCloser, error)
	Type() AdapterType
}

// BackupManager manages backup operations and metrics
type BackupManager struct {
	BackupInterface
	metrics metrics.MetricsCollector
}

// NewBackupManager creates a new BackupManager
func NewBackupManager(backup BackupInterface, metrics metrics.MetricsCollector) *BackupManager {
	return &BackupManager{
		BackupInterface: backup,
		metrics:         metrics,
	}
}

// Create creates a backup and records metrics
func (bm *BackupManager) Create(ctx context.Context, serverRoot, backupDir, name string, ignored []string) (*BackupInfo, error) {
	start := time.Now()
	info, err := bm.BackupInterface.Create(ctx, serverRoot, backupDir, name, ignored)
	if err != nil {
		return nil, err
	}

	bm.metrics.RecordBackupDuration(time.Since(start))
	return info, nil
}

// calculateChecksum computes the SHA-256 hash of a file.
func calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func validNamespace(namespace string) bool {
	if namespace == "" || len(namespace) > 128 || filepath.Base(namespace) != namespace || strings.ContainsAny(namespace, `/\`) {
		return false
	}
	for _, char := range namespace {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func validBackupName(name string) bool {
	if name == "" || len(name) > 128 || !strings.HasSuffix(name, ".zip") || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return false
	}
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func canonicalDirectory(root string, create bool) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("directory is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if create {
		if err := os.MkdirAll(absolute, 0o750); err != nil {
			return "", err
		}
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", canonical)
	}
	return canonical, nil
}
