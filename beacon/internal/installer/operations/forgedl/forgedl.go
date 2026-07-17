package forgedl

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

const forgePromoURL = "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json"
const forgeInstallerURL = "https://maven.minecraftforge.net/net/minecraftforge/forge/%s/forge-%s-installer.jar"

type ForgeDl struct {
	MinecraftVersion string `json:"minecraftVersion"`
	Version          string `json:"version"`
	Filename         string `json:"filename"`
}

func init() {
	operations.Register("forgeDl", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op ForgeDl
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("forgeDl: %w", err)
	}
	if op.Filename == "" {
		op.Filename = "forge-installer.jar"
	}
	if op.Version == "" && op.MinecraftVersion == "" {
		return nil, fmt.Errorf("forgeDl: either version or minecraftVersion is required")
	}
	return &op, nil
}

type forgePromos struct {
	Promos map[string]string `json:"promos"`
}

func (op *ForgeDl) Execute(ctx context.Context, serverDir string) error {
	version := op.Version
	if version == "" {
		latest, err := op.resolveLatest(ctx)
		if err != nil {
			return fmt.Errorf("resolve latest forge: %w", err)
		}
		version = op.MinecraftVersion + "-" + latest
	}

	dlURL := fmt.Sprintf(forgeInstallerURL, version, version)
	dest := operations.ResolvePath(serverDir, op.Filename)
	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	return op.downloadFile(ctx, dlURL, dest)
}

func (op *ForgeDl) resolveLatest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, forgePromoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "GamePanel-Beacon/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET forge promos: status %d", resp.StatusCode)
	}

	var promos forgePromos
	if err := json.NewDecoder(resp.Body).Decode(&promos); err != nil {
		return "", err
	}

	key := op.MinecraftVersion + "-latest"
	version, ok := promos.Promos[key]
	if !ok || version == "" {
		return "", fmt.Errorf("no forge version found for mc %s", op.MinecraftVersion)
	}
	return version, nil
}

func (op *ForgeDl) downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
		return fmt.Errorf("GET %q: status %d", url, resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
