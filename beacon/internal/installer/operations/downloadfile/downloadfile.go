package downloadfile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"gamepanel/beacon/internal/installer/operations"
)

type DownloadFile struct {
	URL     string `json:"url"`
	Dest    string `json:"dest"`
	Timeout int    `json:"timeout,omitempty"`
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

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("write %q: %w", dest, err)
	}
	if written == 0 {
		return fmt.Errorf("downloaded file %q is empty", dest)
	}
	return nil
}
