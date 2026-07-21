package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StorageBackend defines the interface for all storage backends
type StorageBackend interface {
	// Name returns the name of the storage provider
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

// LocalStorageBackend implements StorageBackend for local filesystem storage
type LocalStorageBackend struct {
	basePath string
}

// NewLocalStorageBackend creates a new LocalStorageBackend
func NewLocalStorageBackend(basePath string) (*LocalStorageBackend, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("local storage: invalid path: %w", err)
	}

	if err := os.MkdirAll(abs, 0755); err != nil {
		return nil, fmt.Errorf("local storage: failed to create directory: %w", err)
	}

	return &LocalStorageBackend{basePath: abs}, nil
}

// Name returns the name of the storage backend
func (p *LocalStorageBackend) Name() string {
	return "local"
}

// Upload uploads data to local storage
func (p *LocalStorageBackend) Upload(ctx context.Context, path string, data []byte) error {
	fullPath := filepath.Join(p.basePath, path)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("local storage: failed to create directory: %w", err)
	}

	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return fmt.Errorf("local storage: failed to write file: %w", err)
	}

	return nil
}

// Download downloads data from local storage
func (p *LocalStorageBackend) Download(ctx context.Context, path string) ([]byte, error) {
	fullPath := filepath.Join(p.basePath, path)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("local storage: file not found: %s", path)
		}
		return nil, fmt.Errorf("local storage: failed to read file: %w", err)
	}

	return data, nil
}

// Delete deletes data from local storage
func (p *LocalStorageBackend) Delete(ctx context.Context, path string) error {
	fullPath := filepath.Join(p.basePath, path)

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, consider it deleted
		}
		return fmt.Errorf("local storage: failed to delete file: %w", err)
	}

	return nil
}

// List lists files in local storage with the given prefix
func (p *LocalStorageBackend) List(ctx context.Context, prefix string) ([]string, error) {
	var names []string
	fullPrefix := filepath.Join(p.basePath, prefix)

	entries, err := os.ReadDir(fullPrefix)
	if err != nil {
		if os.IsNotExist(err) {
			return names, nil // Directory doesn't exist, return empty list
		}
		return nil, fmt.Errorf("local storage: failed to list directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}

	return names, nil
}

// Exists checks if a file exists in local storage
func (p *LocalStorageBackend) Exists(ctx context.Context, path string) (bool, error) {
	fullPath := filepath.Join(p.basePath, path)

	_, err := os.Stat(fullPath)
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, fmt.Errorf("local storage: failed to check existence: %w", err)
}

// UploadStream uploads data from a stream to local storage
func (p *LocalStorageBackend) UploadStream(ctx context.Context, path string, reader io.Reader, size int64) error {
	fullPath := filepath.Join(p.basePath, path)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("local storage: failed to create directory: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("local storage: failed to create file: %w", err)
	}
	defer file.Close()

	if size > 0 {
		if err := file.Truncate(size); err != nil {
			return fmt.Errorf("local storage: failed to truncate file: %w", err)
		}
	}

	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("local storage: failed to copy data: %w", err)
	}

	return nil
}

// DownloadStream downloads data from local storage to a stream
func (p *LocalStorageBackend) DownloadStream(ctx context.Context, path string) (io.Reader, error) {
	fullPath := filepath.Join(p.basePath, path)

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("local storage: file not found: %s", path)
		}
		return nil, fmt.Errorf("local storage: failed to open file: %w", err)
	}

	return file, nil
}

// GetFileInfo gets information about a file in local storage
func (p *LocalStorageBackend) GetFileInfo(ctx context.Context, path string) (FileInfo, error) {
	fullPath := filepath.Join(p.basePath, path)

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return FileInfo{}, fmt.Errorf("local storage: file not found: %s", path)
		}
		return FileInfo{}, fmt.Errorf("local storage: failed to get file info: %w", err)
	}

	return FileInfo{
		Name:     info.Name(),
		Path:     path,
		Size:     info.Size(),
		Modified: info.ModTime(),
		IsDir:    info.IsDir(),
	}, nil
}

// StorageManager manages multiple storage backends
type StorageManager struct {
	backends    map[string]StorageBackend
	defaultName string
}

// NewStorageManager creates a new StorageManager
func NewStorageManager() *StorageManager {
	return &StorageManager{
		backends:    make(map[string]StorageBackend),
		defaultName: "local",
	}
}

// RegisterBackend registers a storage backend
func (m *StorageManager) RegisterBackend(name string, backend StorageBackend) {
	m.backends[name] = backend
	if m.defaultName == "" {
		m.defaultName = name
	}
}

// GetBackend gets a storage backend by name
func (m *StorageManager) GetBackend(name string) (StorageBackend, error) {
	if name == "" {
		name = m.defaultName
	}

	backend, exists := m.backends[name]
	if !exists {
		return nil, fmt.Errorf("storage provider not found: %s", name)
	}

	return backend, nil
}

// GetDefaultBackend gets the default storage backend
func (m *StorageManager) GetDefaultBackend() (StorageBackend, error) {
	return m.GetBackend(m.defaultName)
}

// SetDefaultBackend sets the default storage backend
func (m *StorageManager) SetDefaultBackend(name string) error {
	if _, exists := m.backends[name]; !exists {
		return fmt.Errorf("storage provider not found: %s", name)
	}

	m.defaultName = name
	return nil
}

// ListBackends lists all registered storage backends
func (m *StorageManager) ListBackends() []string {
	var names []string
	for name := range m.backends {
		names = append(names, name)
	}
	return names
}

// CreateStoragePath creates a storage path for a backup
func CreateStoragePath(serverID, backupName string, timestamp time.Time) string {
	// Format: backups/{serverID}/{timestamp}_{backupName}.tar.gz
	timestampStr := timestamp.Format("20060102_150405")

	// Sanitize backup name to be filesystem-safe
	safeName := strings.ReplaceAll(backupName, "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")
	safeName = strings.ReplaceAll(safeName, ":", "-")

	return filepath.Join("backups", serverID, fmt.Sprintf("%s_%s.tar.gz", timestampStr, safeName))
}

// ParseStoragePath parses a storage path to extract information
func ParseStoragePath(path string) (serverID string, timestamp time.Time, backupName string, err error) {
	// Expected format: backups/{serverID}/{timestamp}_{backupName}.tar.gz
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		return "", time.Time{}, "", fmt.Errorf("invalid storage path format: %s", path)
	}

	serverID = parts[1]
	filename := parts[2]

	// Remove .tar.gz extension
	if strings.HasSuffix(filename, ".tar.gz") {
		filename = strings.TrimSuffix(filename, ".tar.gz")
	}

	// Split timestamp and backup name
	underscoreIndex := strings.Index(filename, "_")
	if underscoreIndex == -1 {
		return "", time.Time{}, "", fmt.Errorf("invalid filename format: %s", filename)
	}

	timestampStr := filename[:underscoreIndex]
	backupName = filename[underscoreIndex+1:]

	// Parse timestamp
	timestamp, err = time.Parse("20060102_150405", timestampStr)
	if err != nil {
		return "", time.Time{}, "", fmt.Errorf("invalid timestamp format: %s", timestampStr)
	}

	return serverID, timestamp, backupName, nil
}
