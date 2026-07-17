package transfer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gamepanel/beacon/internal/ignore"
)

// Status represents the current state of a transfer
type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusCancelled  Status = "cancelled"
	StatusResuming   Status = "resuming"
)

// Transfer represents a server transfer operation
type Transfer struct {
	ID           string    `json:"id"`
	ServerID     string    `json:"serverId"`
	SourceNode   string    `json:"sourceNode"`
	TargetNode   string    `json:"targetNode"`
	Status       Status    `json:"status"`
	Progress     int       `json:"progress"`
	ArchivePath  string    `json:"archivePath,omitempty"`
	ArchiveSize  int64     `json:"archiveSize,omitempty"`
	Checksum     string    `json:"checksum,omitempty"`
	StartedAt    time.Time `json:"startedAt"`
	CompletedAt  time.Time `json:"completedAt,omitempty"`
	Error        string    `json:"error,omitempty"`
	ResumeOffset int64     `json:"resumeOffset,omitempty"` // For resumable transfers

	mu         sync.Mutex
	cancelFunc context.CancelFunc
}

// Manager manages transfer operations
type Manager struct {
	transfers sync.Map
}

// NewManager creates a new transfer manager
func NewManager() *Manager {
	return &Manager{}
}

// Start begins a new transfer operation
func (m *Manager) Start(_ context.Context, serverID, sourceNode, targetNode, serverRoot, targetURL, token string, resumeOffset int64) (*Transfer, error) {
	if resumeOffset < 0 {
		return nil, fmt.Errorf("resume offset cannot be negative")
	}
	if resumeOffset != 0 {
		return nil, fmt.Errorf("transfer resume is not supported by the destination")
	}
	transferID := fmt.Sprintf("transfer-%s-%d", serverID, time.Now().Unix())

	transferCtx, cancel := context.WithCancel(context.Background())

	transfer := &Transfer{
		ID:           transferID,
		ServerID:     serverID,
		SourceNode:   sourceNode,
		TargetNode:   targetNode,
		Status:       StatusPending,
		StartedAt:    time.Now(),
		cancelFunc:   cancel,
		ResumeOffset: resumeOffset,
	}

	m.transfers.Store(transferID, transfer)

	// Run transfer in background
	go m.executeTransfer(transferCtx, transfer, serverRoot, targetURL, token)

	return transfer, nil
}

// Get returns a transfer by ID
func (m *Manager) Get(transferID string) (*Transfer, bool) {
	value, ok := m.transfers.Load(transferID)
	if !ok {
		return nil, false
	}
	return value.(*Transfer), true
}

// List returns all active transfers
func (m *Manager) List() []*Transfer {
	var transfers []*Transfer
	m.transfers.Range(func(key, value interface{}) bool {
		transfers = append(transfers, value.(*Transfer))
		return true
	})
	return transfers
}

// Cancel cancels an active transfer
func (m *Manager) Cancel(transferID string) error {
	transfer, ok := m.Get(transferID)
	if !ok {
		return fmt.Errorf("transfer not found")
	}

	transfer.mu.Lock()
	defer transfer.mu.Unlock()

	if transfer.Status == StatusProcessing || transfer.Status == StatusPending || transfer.Status == StatusResuming {
		transfer.Status = StatusCancelled
		if transfer.cancelFunc != nil {
			transfer.cancelFunc()
		}
	}

	return nil
}

// executeTransfer performs the actual transfer operation
func (m *Manager) executeTransfer(ctx context.Context, transfer *Transfer, serverRoot, targetURL, token string) {
	transfer.mu.Lock()
	transfer.Status = StatusProcessing
	transfer.mu.Unlock()

	// Create archive
	archivePath, err := m.createArchive(ctx, serverRoot, transfer)
	if err != nil {
		m.failTransfer(transfer, fmt.Errorf("archive creation failed: %w", err))
		return
	}

	transfer.mu.Lock()
	transfer.ArchivePath = archivePath
	transfer.mu.Unlock()

	// Calculate checksum
	checksum, err := m.calculateChecksum(archivePath)
	if err != nil {
		m.failTransfer(transfer, fmt.Errorf("checksum calculation failed: %w", err))
		return
	}

	transfer.mu.Lock()
	transfer.Checksum = checksum
	transfer.mu.Unlock()

	// Stream to target with resume capability
	if err := m.streamToTarget(ctx, archivePath, targetURL, token, transfer); err != nil {
		m.failTransfer(transfer, fmt.Errorf("stream failed: %w", err))
		return
	}

	// Mark complete
	transfer.mu.Lock()
	transfer.Status = StatusCompleted
	transfer.Progress = 100
	transfer.CompletedAt = time.Now()
	transfer.mu.Unlock()

	// Clean up local archive
	os.Remove(archivePath)
}

// createArchive creates a tar.gz archive of the server directory matching
// the format the destination daemon expects (extractTarGzArchive).
func (m *Manager) createArchive(ctx context.Context, serverRoot string, transfer *Transfer) (string, error) {
	archivePath := filepath.Join(os.TempDir(), fmt.Sprintf("transfer-%s.tar.gz", transfer.ID))

	file, err := os.Create(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to create archive file: %w", err)
	}
	defer file.Close()

	gz := gzip.NewWriter(file)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	// Load ignore patterns (matches .pteroignore if present)
	denylist, _ := ignore.LoadServerIgnore(serverRoot)

	var totalFiles int64
	var processedFiles int64

	// Count total files
	err = filepath.WalkDir(serverRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			totalFiles++
		}
		return nil
	})
	if err != nil {
		os.Remove(archivePath)
		return "", fmt.Errorf("failed to count files: %w", err)
	}

	// Create archive
	err = filepath.WalkDir(serverRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rel, err := filepath.Rel(serverRoot, path)
		if err != nil || rel == "." {
			return err
		}

		first := strings.Split(filepath.ToSlash(rel), "/")[0]
		if first == ".backups" || first == ".uploads" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if denylist != nil && (denylist.IsIgnored(rel) || denylist.IsIgnored(entry.Name())) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("failed to get file info for %s: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header for %s: %w", path, err)
		}
		header.Name = filepath.ToSlash(rel)

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header for %s: %w", path, err)
		}

		source, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}
		defer source.Close()

		if _, err := io.Copy(tw, source); err != nil {
			return fmt.Errorf("failed to copy file %s to archive: %w", path, err)
		}

		processedFiles++
		progress := int(float64(processedFiles) / float64(max(totalFiles, 1)) * 50)
		transfer.mu.Lock()
		transfer.Progress = progress
		transfer.mu.Unlock()

		return nil
	})

	if err != nil {
		os.Remove(archivePath)
		return "", fmt.Errorf("failed to create archive: %w", err)
	}
	if err := tw.Close(); err != nil {
		os.Remove(archivePath)
		return "", fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gz.Close(); err != nil {
		os.Remove(archivePath)
		return "", fmt.Errorf("failed to close gzip writer: %w", err)
	}

	info, err := os.Stat(archivePath)
	if err != nil {
		os.Remove(archivePath)
		return "", fmt.Errorf("failed to get archive file info: %w", err)
	}

	transfer.mu.Lock()
	transfer.ArchiveSize = info.Size()
	transfer.mu.Unlock()

	return archivePath, nil
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// calculateChecksum computes SHA256 hash of the archive
func (m *Manager) calculateChecksum(archivePath string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to open archive for checksum: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to calculate checksum: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// streamToTarget uploads the archive to the target node with resume capability
func (m *Manager) streamToTarget(ctx context.Context, archivePath, targetURL, token string, transfer *Transfer) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive for upload: %w", err)
	}
	defer file.Close()

	// Get file info for size and offset
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get archive info: %w", err)
	}

	offset := transfer.ResumeOffset
	if offset < 0 || offset > info.Size() {
		return fmt.Errorf("resume offset %d is outside archive size %d", offset, info.Size())
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek archive to resume offset %d: %w", offset, err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, file)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Transfer-ID", transfer.ID)
	req.Header.Set("X-Transfer-ServerID", transfer.ServerID)
	req.Header.Set("X-Checksum", transfer.Checksum)
	req.Header.Set("X-Transfer-Size", fmt.Sprintf("%d", info.Size()))
	req.Header.Set("X-Transfer-Resume-Offset", fmt.Sprintf("%d", offset))

	// Set content length
	req.ContentLength = info.Size() - offset

	// Create client with timeout
	client := &http.Client{
		Timeout: 30 * time.Minute,
	}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send upload request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		return fmt.Errorf("target node returned error: %s - %s", resp.Status, strings.TrimSpace(string(body)))
	}

	// Update progress
	transfer.mu.Lock()
	transfer.Progress = 100
	transfer.mu.Unlock()

	return nil
}

// failTransfer marks a transfer as failed
func (m *Manager) failTransfer(transfer *Transfer, err error) {
	transfer.mu.Lock()
	defer transfer.mu.Unlock()

	transfer.Status = StatusFailed
	transfer.Error = err.Error()
	transfer.CompletedAt = time.Now()

	// Clean up archive
	if transfer.ArchivePath != "" {
		os.Remove(transfer.ArchivePath)
	}
}
