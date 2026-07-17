package fabricdl

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

const fabricMetaURL = "https://meta.fabricmc.net/v2/versions/installer"

type FabricDl struct {
	Filename string `json:"filename"`
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
	dest := operations.ResolvePath(serverDir, op.Filename)
	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "GamePanel-Beacon/1.0")

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %q: status %d", dlURL, resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (op *FabricDl) fetchInstallers(ctx context.Context) ([]fabricInstallerInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fabricMetaURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GamePanel-Beacon/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET fabric meta: status %d", resp.StatusCode)
	}

	var installers []fabricInstallerInfo
	if err := json.NewDecoder(resp.Body).Decode(&installers); err != nil {
		return nil, err
	}
	return installers, nil
}
