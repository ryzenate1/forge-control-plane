package server

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxUpgradeDownloadSize = 512 << 20

var upgradeHTTPClient = &http.Client{
	Timeout: 15 * time.Minute,
	Transport: &http.Transport{
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many upgrade download redirects")
		}
		return validateUpgradeURL(req.URL)
	},
}

var (
	ErrUpgradeInProgress   = errors.New("upgrade already in progress")
	ErrUpgradeNotStarted   = errors.New("no upgrade to roll back")
	ErrVersionIncompatible = errors.New("version incompatible")
)

type UpgradePayload struct {
	Version     string `json:"version"`
	DownloadURL string `json:"downloadUrl"`
	Checksum    string `json:"checksum"`
	Signature   string `json:"signature"`
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
	if strings.TrimSpace(payload.Version) == "" {
		return errors.New("upgrade version is required")
	}
	compat := CheckVersionCompatibility(payload.Version, "")
	if _, err := compareReleaseVersions(payload.Version, "0.0.0"); err != nil {
		return fmt.Errorf("invalid upgrade version: %w", err)
	}
	if !payload.Force && !compat.Compatible {
		return ErrVersionIncompatible
	}
	parsedURL, err := url.Parse(payload.DownloadURL)
	if err != nil {
		return fmt.Errorf("invalid upgrade download URL: %w", err)
	}
	if err := validateUpgradeURL(parsedURL); err != nil {
		return err
	}
	payload.Checksum = strings.TrimPrefix(strings.TrimSpace(payload.Checksum), "sha256:")
	decodedChecksum, err := hex.DecodeString(payload.Checksum)
	if err != nil || len(decodedChecksum) != sha256.Size {
		return errors.New("a valid SHA-256 checksum is required for upgrades")
	}
	if err := verifyUpgradeAuthorization(payload); err != nil {
		return err
	}

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

	if err := os.MkdirAll(m.upgradeDir, 0o700); err != nil {
		m.fail(err)
		return err
	}
	if err := os.Remove(filepath.Join(m.upgradeDir, "beacon.new")); err != nil && !errors.Is(err, os.ErrNotExist) {
		m.fail(err)
		return fmt.Errorf("remove stale staged upgrade: %w", err)
	}
	if err := os.Remove(filepath.Join(m.upgradeDir, "beacon.download")); err != nil && !errors.Is(err, os.ErrNotExist) {
		m.fail(err)
		return fmt.Errorf("remove stale upgrade download: %w", err)
	}

	if err := m.backupCurrentBinary(); err != nil {
		m.fail(fmt.Errorf("backup failed: %w", err))
		return err
	}

	if err := m.downloadAndVerify(ctx, payload); err != nil {
		m.fail(err)
		return err
	}

	return nil
}

func verifyUpgradeAuthorization(payload UpgradePayload) error {
	publicKeyEncoded := strings.TrimSpace(os.Getenv("DAEMON_UPGRADE_PUBLIC_KEY"))
	if publicKeyEncoded == "" {
		return errors.New("DAEMON_UPGRADE_PUBLIC_KEY is required to authorize upgrades")
	}
	publicKey, err := base64.StdEncoding.DecodeString(publicKeyEncoded)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("DAEMON_UPGRADE_PUBLIC_KEY must be a base64 Ed25519 public key")
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload.Signature))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("upgrade signature must be a base64 Ed25519 signature")
	}
	message := payload.Version + "\n" + payload.DownloadURL + "\n" + strings.ToLower(payload.Checksum)
	if !ed25519.Verify(ed25519.PublicKey(publicKey), []byte(message), signature) {
		return errors.New("upgrade publisher signature is invalid")
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
	if err := ctx.Err(); err != nil {
		m.fail(err)
		return err
	}
	if err := atomicReplaceFromFile(newBin, target, 0o755); err != nil {
		m.fail(fmt.Errorf("atomic apply failed: %w", err))
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

	backupPath := m.backupPath(m.prevVersion)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		m.mu.Lock()
		m.state = UpgradeStateRolledBack
		m.err = fmt.Sprintf("backup binary not found at %s", backupPath)
		m.completedAt = time.Now().UTC()
		m.mu.Unlock()
		return fmt.Errorf("backup binary not found at %s", backupPath)
	}

	if err := ctx.Err(); err != nil {
		m.fail(err)
		return err
	}
	if prevState == UpgradeStateCompleted {
		if err := atomicReplaceFromFile(m.binPath, m.binPath+".failed", 0o700); err != nil {
			m.fail(fmt.Errorf("preserve failed binary: %w", err))
			return err
		}
	}
	if err := atomicReplaceFromFile(backupPath, m.binPath, 0o755); err != nil {
		m.fail(fmt.Errorf("rollback copy failed: %w", err))
		return err
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
	if err := os.MkdirAll(m.backupDir, 0o700); err != nil {
		return err
	}
	backupPath := m.backupPath(m.currentVersion)
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	}
	return atomicReplaceFromFile(m.binPath, backupPath, 0o700)
}

func (m *UpgradeManager) backupPath(version string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, version)
	safe = strings.Trim(safe, ".")
	if safe == "" {
		sum := sha256.Sum256([]byte(version))
		safe = hex.EncodeToString(sum[:8])
	}
	return filepath.Join(m.backupDir, "beacon."+safe)
}

func (m *UpgradeManager) fail(err error) {
	m.mu.Lock()
	m.state = UpgradeStateFailed
	m.err = err.Error()
	m.completedAt = time.Now().UTC()
	m.mu.Unlock()
}

func (m *UpgradeManager) downloadAndVerify(ctx context.Context, payload UpgradePayload) error {
	m.mu.Lock()
	m.progress = "downloading"
	m.progressPct = 10
	m.mu.Unlock()

	downloadPath := filepath.Join(m.upgradeDir, "beacon.download")
	out, err := os.OpenFile(downloadPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create download file: %w", err)
	}
	defer out.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, payload.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := upgradeHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download returned %s", resp.Status)
	}
	if resp.ContentLength > maxUpgradeDownloadSize {
		return fmt.Errorf("upgrade download is too large: %d bytes", resp.ContentLength)
	}

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(out, hasher), io.LimitReader(resp.Body, maxUpgradeDownloadSize+1))
	if err != nil {
		return fmt.Errorf("download write: %w", err)
	}
	if written > maxUpgradeDownloadSize {
		_ = os.Remove(downloadPath)
		return errors.New("upgrade download exceeded maximum size")
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync upgrade download: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close upgrade download: %w", err)
	}

	m.mu.Lock()
	m.progress = "verifying checksum"
	m.progressPct = 70
	m.mu.Unlock()

	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actual, payload.Checksum) {
		os.Remove(downloadPath)
		return fmt.Errorf("checksum mismatch: expected %s, got %s", payload.Checksum, actual)
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

func validateUpgradeURL(parsed *url.URL) error {
	if parsed == nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return errors.New("upgrade download URL must use HTTPS")
	}
	if parsed.User != nil {
		return errors.New("upgrade download URL must not contain credentials")
	}
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
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	if header[0] == 0x1f && header[1] == 0x8b {
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gzr.Close()
		tr := tar.NewReader(gzr)
		found := false
		tempPath := outputPath + ".extracting"
		_ = os.Remove(tempPath)
		defer os.Remove(tempPath)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			name := filepath.Base(filepath.Clean(hdr.Name))
			if hdr.Typeflag == tar.TypeReg && (name == "beacon" || name == "gamepanel-beacon") {
				if found {
					return errors.New("archive contains multiple Beacon binaries")
				}
				if hdr.Size <= 0 || hdr.Size > maxUpgradeDownloadSize {
					return errors.New("archive Beacon binary has an invalid size")
				}
				out, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
				if err != nil {
					return err
				}
				written, copyErr := io.Copy(out, io.LimitReader(tr, maxUpgradeDownloadSize+1))
				err = copyErr
				if err == nil && written != hdr.Size {
					err = errors.New("archive Beacon binary was truncated")
				}
				if err == nil {
					err = out.Sync()
				}
				closeErr := out.Close()
				if err != nil {
					os.Remove(tempPath)
					return err
				}
				if closeErr != nil {
					os.Remove(tempPath)
					return closeErr
				}
				found = true
			}
		}
		if !found {
			return errors.New("no Beacon binary found in archive")
		}
		if err := os.Chmod(tempPath, 0o755); err != nil {
			return err
		}
		return os.Rename(tempPath, outputPath)
	}

	if err := os.Remove(outputPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(archivePath, outputPath); err != nil {
		return copyFile(archivePath, outputPath)
	}
	return nil
}

func copyFile(src, dst string) error {
	return copyFileSynced(src, dst, 0o600)
}

func copyFileSynced(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func atomicReplaceFromFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	out, err := os.CreateTemp(dir, "."+filepath.Base(dst)+".stage-*")
	if err != nil {
		return err
	}
	staged := out.Name()
	defer os.Remove(staged)
	if err := out.Chmod(mode); err != nil {
		_ = out.Close()
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Rename(staged, dst); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func syncDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
