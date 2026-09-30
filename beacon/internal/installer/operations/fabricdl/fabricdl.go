package fabricdl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gamepanel/beacon/internal/installer/operations"
)

const fabricMetaURL = "https://meta.fabricmc.net/v2/versions/installer"

type FabricDl struct {
	Filename       string `json:"filename"`
	ExpectedSHA256 string `json:"expectedSha256"`
}

func init() {
	operations.Register("fabricDl", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op FabricDl
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("fabricDl: %w", err)
	}
	if op.Filename == "" {
		op.Filename = "fabric-installer.jar"
	}
	if decoded, err := hex.DecodeString(op.ExpectedSHA256); err != nil || len(decoded) != sha256.Size {
		return nil, fmt.Errorf("fabricDl: expectedSha256 is required")
	}
	return &op, nil
}

type fabricInstallerInfo struct {
	URL string `json:"url"`
}

func (op *FabricDl) Execute(ctx context.Context, serverDir string) error {
	installers, err := op.fetchInstallers(ctx)
	if err != nil {
		return fmt.Errorf("fetch fabric installers: %w", err)
	}
	if len(installers) == 0 {
		return fmt.Errorf("no fabric installers available")
	}

	dlURL := installers[0].URL
	dest, err := operations.ResolvePath(serverDir, op.Filename)
	if err != nil {
		return err
	}
	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	return operations.DownloadVerified(ctx, dlURL, dest, op.ExpectedSHA256, 2<<30, 10*time.Minute)
}

func (op *FabricDl) fetchInstallers(ctx context.Context) ([]fabricInstallerInfo, error) {
	client := operations.SecureHTTPClient(30 * time.Second)
	resp, err := operations.DoWithRetry(ctx, client, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, fabricMetaURL, nil)
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET fabric meta: status %d", resp.StatusCode)
	}

	var installers []fabricInstallerInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&installers); err != nil {
		return nil, err
	}
	return installers, nil
}
