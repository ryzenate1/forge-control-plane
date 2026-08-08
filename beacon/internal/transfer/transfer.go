package transfer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
	ArchivePath  string    `json:"-"`
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
	ctx       context.Context
	cancel    context.CancelFunc
	sem       chan struct{}
	tempRoot  string
	initErr   error
	client    *http.Client
	wg        sync.WaitGroup
}

// NewManager creates a new transfer manager
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	tempRoot, err := os.MkdirTemp("", "gamepanel-transfers-")
	if err == nil {
		err = os.Chmod(tempRoot, 0o700)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ExpectContinueTimeout = 5 * time.Second
	return &Manager{
		ctx:      ctx,
		cancel:   cancel,
		sem:      make(chan struct{}, 4),
		tempRoot: tempRoot,
		initErr:  err,
		client: &http.Client{
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return validateTargetURL(req.URL)
			},
		},
	}
}

// Start begins a new transfer operation
func (m *Manager) Start(_ context.Context, serverID, sourceNode, targetNode, serverRoot, targetURL, token string, resumeOffset int64) (*Transfer, error) {
	if m.initErr != nil {
		return nil, fmt.Errorf("initialize private transfer directory: %w", m.initErr)
	}
	if err := m.ctx.Err(); err != nil {
		return nil, errors.New("transfer manager is closed")
	}
	if token == "" {
		return nil, errors.New("transfer token is required")
	}
	if resumeOffset < 0 {
		return nil, fmt.Errorf("resume offset cannot be negative")
	}
	if resumeOffset != 0 {
		return nil, fmt.Errorf("legacy transfer resume offset requires zero; resumable migrations use protocol v1")
	}
	parsedTarget, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("parse target URL: %w", err)
	}
	if err := validateTargetURL(parsedTarget); err != nil {
		return nil, err
	}
	canonicalRoot, err := filepath.EvalSymlinks(serverRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve server root: %w", err)
	}
	info, err := os.Stat(canonicalRoot)
	if err != nil || !info.IsDir() {
		return nil, errors.New("server root must be an existing directory")
	}
	select {
	case m.sem <- struct{}{}:
	default:
		return nil, errors.New("transfer concurrency limit reached")
	}
	randomID := make([]byte, 16)
	if _, err := rand.Read(randomID); err != nil {
		<-m.sem
		return nil, fmt.Errorf("generate transfer ID: %w", err)
	}
	transferID := "transfer-" + hex.EncodeToString(randomID)

	transferCtx, cancel := context.WithCancel(m.ctx)

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
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() { <-m.sem }()
		m.executeTransfer(transferCtx, transfer, canonicalRoot, parsedTarget.String(), token)
	}()

	return transfer, nil
}

// Close cancels all active transfers, waits for workers to exit, and removes
// the private temporary directory.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.cancel()
	m.wg.Wait()
	if m.tempRoot == "" {
		return nil
	}
	return os.RemoveAll(m.tempRoot)
}

// Get returns a transfer by ID
func (m *Manager) Get(transferID string) (*Transfer, bool) {
	value, ok := m.transfers.Load(transferID)
	if !ok {
		return nil, false
	}
	t, ok := value.(*Transfer)
	if !ok {
		return nil, false
	}
	return t, true
}

// List returns all active transfers
func (m *Manager) List() []*Transfer {
	var transfers []*Transfer
	m.transfers.Range(func(key, value interface{}) bool {
		if t, ok := value.(*Transfer); ok {
			transfers = append(transfers, t)
		}
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
	file, err := os.CreateTemp(m.tempRoot, "archive-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("failed to create archive file: %w", err)
	}
	archivePath := file.Name()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(archivePath)
		return "", fmt.Errorf("secure archive permissions: %w", err)
	}
	defer file.Close()

	gz := gzip.NewWriter(file)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	// Load ignore patterns (matches .pteroignore if present)
	denylist, err := ignore.LoadServerIgnore(serverRoot)
	if err != nil {
		_ = os.Remove(archivePath)
		return "", fmt.Errorf("load server ignore rules: %w", err)
	}

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

		if _, err := io.Copy(tw, source); err != nil {
			source.Close()
			return fmt.Errorf("failed to copy file %s to archive: %w", path, err)
		}
		source.Close()

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
	if err := file.Sync(); err != nil {
		os.Remove(archivePath)
		return "", fmt.Errorf("failed to sync archive: %w", err)
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

	// Send request
	resp, err := m.client.Do(req)
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

func validateTargetURL(target *url.URL) error {
	if target == nil || target.Hostname() == "" {
		return errors.New("target URL must include a host")
	}
	if target.User != nil {
		return errors.New("target URL must not include credentials")
	}
	if target.Scheme == "https" {
		return nil
	}
	ip := net.ParseIP(target.Hostname())
	if target.Scheme == "http" && (strings.EqualFold(target.Hostname(), "localhost") || ip != nil && ip.IsLoopback()) {
		return nil
	}
	return errors.New("target URL must use HTTPS (HTTP is allowed only for loopback)")
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
