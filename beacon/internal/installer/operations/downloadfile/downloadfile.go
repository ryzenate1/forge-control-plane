package downloadfile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
	return &op, nil
}

func (op *DownloadFile) Execute(ctx context.Context, serverDir string) error {
	dest := operations.ResolvePath(serverDir, op.Dest)
	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	client := &http.Client{Timeout: time.Duration(op.Timeout) * time.Second}
	if op.Timeout <= 0 {
		client.Timeout = 10 * time.Minute
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, op.URL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("get %q: %w", op.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %q: unexpected status %d", op.URL, resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %q: %w", dest, err)
	}
	defer out.Close()

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(out, hasher), resp.Body)
	if err != nil {
		return fmt.Errorf("write %q: %w", dest, err)
	}
	if written == 0 {
		return fmt.Errorf("downloaded file %q is empty", dest)
	}

	if op.ExpectedSHA256 == "" {
		log.Printf("[installer] warning: downloadFile %q has no expectedSha256; integrity verification skipped", op.URL)
		return nil
	}

	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, op.ExpectedSHA256) {
		_ = out.Close()
		_ = os.Remove(dest)
		return fmt.Errorf("downloaded file %q sha256 mismatch: got %s, expected %s", dest, got, op.ExpectedSHA256)
	}
	return nil
}
