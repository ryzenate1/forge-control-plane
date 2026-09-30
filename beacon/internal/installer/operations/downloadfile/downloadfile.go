package downloadfile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gamepanel/beacon/internal/installer/operations"
)

type DownloadFile struct {
	URL     string `json:"url"`
	Dest    string `json:"dest"`
	Timeout int    `json:"timeout,omitempty"`
	// ExpectedSHA256 is an optional hex-encoded SHA-256 digest of the
	// downloaded content. When set, the download is verified after it
	// completes and is deleted (with the whole operation failing) if the
	// digest does not match. When empty, integrity verification is skipped
	// and a warning is logged, since some legitimate installer definitions
	// may not have a checksum available.
	ExpectedSHA256 string `json:"expectedSha256,omitempty"`
	MaxBytes       int64  `json:"maxBytes,omitempty"`
	client         *http.Client
}

func init() {
	operations.Register("downloadFile", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op DownloadFile
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("downloadFile: %w", err)
	}
	if op.URL == "" || op.Dest == "" {
		return nil, fmt.Errorf("downloadFile: url and dest are required")
	}
	decoded, err := hex.DecodeString(op.ExpectedSHA256)
	if err != nil || len(decoded) != sha256.Size {
		return nil, fmt.Errorf("downloadFile: expectedSha256 must be a 64-character SHA-256 digest")
	}
	if op.MaxBytes <= 0 {
		op.MaxBytes = 4 << 30
	}
	return &op, nil
}

func (op *DownloadFile) Execute(ctx context.Context, serverDir string) error {
	if op.MaxBytes <= 0 {
		op.MaxBytes = 4 << 30
	}
	decoded, err := hex.DecodeString(op.ExpectedSHA256)
	if err != nil || len(decoded) != sha256.Size {
		return errors.New("expectedSha256 must be a valid SHA-256 digest")
	}
	dest, err := operations.ResolvePath(serverDir, op.Dest)
	if err != nil {
		return err
	}
	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	timeout := time.Duration(op.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	client := op.client
	if client == nil {
		client = operations.SecureHTTPClient(timeout)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, op.URL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if op.client == nil {
		if err := operations.ValidateDownloadURL(req.URL); err != nil {
			return fmt.Errorf("validate download URL: %w", err)
		}
	}

	resp, err := operations.DoWithRetry(ctx, client, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, op.URL, nil)
	})
	if err != nil {
		return fmt.Errorf("get %q: %w", op.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %q: unexpected status %d", op.URL, resp.StatusCode)
	}
	if resp.ContentLength > op.MaxBytes {
		return fmt.Errorf("download exceeds %d-byte limit", op.MaxBytes)
	}
	requiredSpace := op.MaxBytes
	if resp.ContentLength > 0 {
		requiredSpace = resp.ContentLength
	}
	if err := operations.EnsureDiskSpace(filepath.Dir(dest), requiredSpace); err != nil {
		return err
	}

	out, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".download-*")
	if err != nil {
		return fmt.Errorf("create %q: %w", dest, err)
	}
	tempPath := out.Name()
	defer os.Remove(tempPath)
	if err := out.Chmod(0o600); err != nil {
		_ = out.Close()
		return err
	}

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(out, hasher), io.LimitReader(resp.Body, op.MaxBytes+1))
	if err != nil {
		_ = out.Close()
		return fmt.Errorf("write %q: %w", dest, err)
	}
	if written == 0 || written > op.MaxBytes {
		_ = out.Close()
		return fmt.Errorf("downloaded file %q is empty", dest)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, op.ExpectedSHA256) {
		return fmt.Errorf("downloaded file %q sha256 mismatch: got %s, expected %s", dest, got, op.ExpectedSHA256)
	}
	if err := os.Rename(tempPath, dest); err != nil {
		return err
	}
	return operations.SyncDirectory(filepath.Dir(dest))
}
