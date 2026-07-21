package server

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrUpgradeInProgress   = errors.New("upgrade already in progress")
	ErrUpgradeNotStarted   = errors.New("no upgrade to roll back")
	ErrVersionIncompatible = errors.New("version incompatible")
)

type UpgradePayload struct {
	Version     string `json:"version"`
	DownloadURL string `json:"downloadUrl"`
	Checksum    string `json:"checksum"`
	Force       bool   `json:"force"`
}

type UpgradeState string

const (
	UpgradeStateIdle        UpgradeState = "idle"
	UpgradeStateDownloading UpgradeState = "downloading"
	UpgradeStateVerifying   UpgradeState = "verifying"
	UpgradeStateApplying    UpgradeState = "applying"
	UpgradeStateCompleted   UpgradeState = "completed"
	UpgradeStateFailed      UpgradeState = "failed"
	UpgradeStateRollingBack UpgradeState = "rollingBack"
	UpgradeStateRolledBack  UpgradeState = "rolledBack"
)

type UpgradeStatus struct {
	State       UpgradeState `json:"state"`
	Version     string       `json:"version"`
	PrevVersion string       `json:"prevVersion"`
	StartedAt   time.Time    `json:"startedAt"`
	CompletedAt time.Time    `json:"completedAt,omitempty"`
	Error       string       `json:"error,omitempty"`
	Progress    string       `json:"progress,omitempty"`
	ProgressPct int          `json:"progressPct"`
}

type UpgradeManager struct {
	currentVersion string
	currentHash    string
	upgradeDir     string
	binPath        string
	backupDir      string

	mu          sync.Mutex
	state       UpgradeState
	version     string
	prevVersion string
	startedAt   time.Time
	completedAt time.Time
	err         string
	progress    string
	progressPct int
}

func NewUpgradeManager(currentVersion, binPath, dataDir string) *UpgradeManager {
	return &UpgradeManager{
		currentVersion: currentVersion,
		binPath:        binPath,
		upgradeDir:     filepath.Join(dataDir, "upgrades"),
		backupDir:      filepath.Join(dataDir, "backups", "beacon"),
		state:          UpgradeStateIdle,
	}
}

func (m *UpgradeManager) Status() UpgradeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return UpgradeStatus{
		State:       m.state,
		Version:     m.version,
		PrevVersion: m.prevVersion,
		StartedAt:   m.startedAt,
		CompletedAt: m.completedAt,
		Error:       m.err,
		Progress:    m.progress,
		ProgressPct: m.progressPct,
	}
}

func (m *UpgradeManager) Begin(ctx context.Context, payload UpgradePayload) error {
	m.mu.Lock()
	if m.state == UpgradeStateDownloading || m.state == UpgradeStateApplying {
		m.mu.Unlock()
		return ErrUpgradeInProgress
	}
	m.state = UpgradeStateDownloading
	m.version = payload.Version
	m.prevVersion = m.currentVersion
	m.startedAt = time.Now().UTC()
	m.err = ""
	m.progress = "starting upgrade"
	m.progressPct = 0
	m.mu.Unlock()

	if !payload.Force {
		compat := CheckVersionCompatibility(payload.Version, "")
		if !compat.Compatible {
			m.mu.Lock()
			m.state = UpgradeStateFailed
			m.err = compat.Message
			m.completedAt = time.Now().UTC()
			m.mu.Unlock()
			return ErrVersionIncompatible
		}
	}

	if err := os.MkdirAll(m.upgradeDir, 0o750); err != nil {
		m.mu.Lock()
		m.state = UpgradeStateFailed
		m.err = err.Error()
		m.completedAt = time.Now().UTC()
		m.mu.Unlock()
		return err
	}

	if err := m.backupCurrentBinary(); err != nil {
		m.mu.Lock()
		m.state = UpgradeStateFailed
		m.err = fmt.Sprintf("backup failed: %v", err)
		m.completedAt = time.Now().UTC()
		m.mu.Unlock()
		return err
	}

	if err := m.downloadAndVerify(ctx, payload); err != nil {
		m.mu.Lock()
		m.state = UpgradeStateFailed
		m.err = err.Error()
		m.completedAt = time.Now().UTC()
		m.mu.Unlock()
		return err
	}

	return nil
}

func (m *UpgradeManager) Apply(ctx context.Context) error {
	m.mu.Lock()
	if m.state != UpgradeStateVerifying {
		m.mu.Unlock()
		return errors.New("upgrade not ready to apply")
	}
	m.state = UpgradeStateApplying
	m.progress = "applying upgrade"
	m.progressPct = 50
	m.mu.Unlock()

	newBin := filepath.Join(m.upgradeDir, "beacon.new")
	target := m.binPath

	if err := os.Rename(newBin, target); err != nil {
		m.mu.Lock()
		m.state = UpgradeStateFailed
		m.err = fmt.Sprintf("apply failed: %v", err)
		m.completedAt = time.Now().UTC()
		m.mu.Unlock()
		return err
	}

	if err := os.Chmod(target, 0o755); err != nil {
		m.mu.Lock()
		m.state = UpgradeStateFailed
		m.err = fmt.Sprintf("chmod failed: %v", err)
		m.completedAt = time.Now().UTC()
		m.mu.Unlock()
		return err
	}

	m.mu.Lock()
	m.state = UpgradeStateCompleted
	m.currentVersion = m.version
	m.currentHash = ""
	m.progress = "upgrade applied successfully"
	m.progressPct = 100
	m.completedAt = time.Now().UTC()
	m.mu.Unlock()

	log.Printf("[upgrade] beacon upgraded to version %s", m.version)
	return nil
}

func (m *UpgradeManager) Rollback(ctx context.Context) error {
	m.mu.Lock()
	if m.state != UpgradeStateFailed && m.state != UpgradeStateCompleted {
		m.mu.Unlock()
		return errors.New("only failed or completed upgrades can be rolled back")
	}
	prevState := m.state
	m.state = UpgradeStateRollingBack
	m.progress = "rolling back"
	m.progressPct = 0
	m.mu.Unlock()

	backupPath := filepath.Join(m.backupDir, fmt.Sprintf("beacon.%s", m.prevVersion))
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		m.mu.Lock()
		m.state = UpgradeStateRolledBack
		m.err = fmt.Sprintf("backup binary not found at %s", backupPath)
		m.completedAt = time.Now().UTC()
		m.mu.Unlock()
		return fmt.Errorf("backup binary not found at %s", backupPath)
	}

	if prevState == UpgradeStateCompleted {
		if err := os.Rename(m.binPath, m.binPath+".failed"); err != nil {
			log.Printf("[upgrade] warning: could not rename failed binary: %v", err)
		}
	}

	target := m.binPath
	if err := copyFile(backupPath, target); err != nil {
		m.mu.Lock()
		m.state = UpgradeStateFailed
		m.err = fmt.Sprintf("rollback copy failed: %v", err)
		m.completedAt = time.Now().UTC()
		m.mu.Unlock()
		return err
	}

	if err := os.Chmod(target, 0o755); err != nil {
		log.Printf("[upgrade] warning: chmod after rollback: %v", err)
	}

	m.mu.Lock()
	m.state = UpgradeStateRolledBack
	m.currentVersion = m.prevVersion
	m.progress = fmt.Sprintf("rolled back to version %s", m.prevVersion)
	m.progressPct = 100
	m.completedAt = time.Now().UTC()
	m.mu.Unlock()

	log.Printf("[upgrade] beacon rolled back to version %s", m.prevVersion)
	return nil
}

func (m *UpgradeManager) backupCurrentBinary() error {
	if err := os.MkdirAll(m.backupDir, 0o750); err != nil {
		return err
	}
	backupPath := filepath.Join(m.backupDir, fmt.Sprintf("beacon.%s", m.currentVersion))
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	}
	return copyFile(m.binPath, backupPath)
}

func (m *UpgradeManager) downloadAndVerify(ctx context.Context, payload UpgradePayload) error {
	m.mu.Lock()
	m.progress = "downloading"
	m.progressPct = 10
	m.mu.Unlock()

	downloadPath := filepath.Join(m.upgradeDir, "beacon.download")
	out, err := os.Create(downloadPath)
	if err != nil {
		return fmt.Errorf("create download file: %w", err)
	}
	defer out.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, payload.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download returned %s", resp.Status)
	}

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(out, hasher), resp.Body)
	if err != nil {
		return fmt.Errorf("download write: %w", err)
	}
	out.Close()

	m.mu.Lock()
	m.progress = "verifying checksum"
	m.progressPct = 70
	m.mu.Unlock()

	if payload.Checksum != "" {
		actual := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(actual, payload.Checksum) {
			os.Remove(downloadPath)
			return fmt.Errorf("checksum mismatch: expected %s, got %s", payload.Checksum, actual)
		}
	}

	if err := extractBinary(downloadPath, filepath.Join(m.upgradeDir, "beacon.new")); err != nil {
		os.Remove(downloadPath)
		return fmt.Errorf("extract binary: %w", err)
	}
	os.Remove(downloadPath)

	m.mu.Lock()
	m.state = UpgradeStateVerifying
	m.progress = fmt.Sprintf("downloaded %d bytes, checksum verified", written)
	m.progressPct = 80
	m.mu.Unlock()

	return nil
}

func extractBinary(archivePath, outputPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	header := make([]byte, 2)
	if _, err := f.Read(header); err != nil {
		return err
	}
	f.Seek(0, 0)

	if header[0] == 0x1f && header[1] == 0x8b {
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gzr.Close()
		tr := tar.NewReader(gzr)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if hdr.Typeflag == tar.TypeReg && (strings.Contains(hdr.Name, "beacon") || strings.Contains(hdr.Name, "gamepanel-beacon")) {
				out, err := os.Create(outputPath)
				if err != nil {
					return err
				}
				_, err = io.Copy(out, tr)
				out.Close()
				if err != nil {
					os.Remove(outputPath)
					return err
				}
				return os.Chmod(outputPath, 0o755)
			}
		}
		return errors.New("no beacon binary found in archive")
	}

	if err := os.Rename(archivePath, outputPath); err != nil {
		return copyFile(archivePath, outputPath)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
