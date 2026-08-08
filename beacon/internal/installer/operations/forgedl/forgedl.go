package forgedl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"gamepanel/beacon/internal/installer/operations"
)

const forgePromoURL = "https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json"
const forgeInstallerURL = "https://maven.minecraftforge.net/net/minecraftforge/forge/%s/forge-%s-installer.jar"

type ForgeDl struct {
	MinecraftVersion string `json:"minecraftVersion"`
	Version          string `json:"version"`
	Filename         string `json:"filename"`
	ExpectedSHA256   string `json:"expectedSha256"`
}

var forgeVersionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,127}$`)

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
	if op.Version != "" && !forgeVersionPattern.MatchString(op.Version) {
		return nil, fmt.Errorf("forgeDl: invalid version")
	}
	if op.MinecraftVersion != "" && !forgeVersionPattern.MatchString(op.MinecraftVersion) {
		return nil, fmt.Errorf("forgeDl: invalid minecraftVersion")
	}
	if op.Version != "" && op.MinecraftVersion != "" && !strings.HasPrefix(op.Version, op.MinecraftVersion+"-") {
		return nil, fmt.Errorf("forgeDl: version does not belong to minecraftVersion")
	}
	if decoded, err := hex.DecodeString(op.ExpectedSHA256); err != nil || len(decoded) != sha256.Size {
		return nil, fmt.Errorf("forgeDl: expectedSha256 is required")
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
	dest, err := operations.ResolvePath(serverDir, op.Filename)
	if err != nil {
		return err
	}
	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	return operations.DownloadVerified(ctx, dlURL, dest, op.ExpectedSHA256, 2<<30, 10*time.Minute)
}

func (op *ForgeDl) resolveLatest(ctx context.Context) (string, error) {
	client := operations.SecureHTTPClient(30 * time.Second)
	resp, err := operations.DoWithRetry(ctx, client, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, forgePromoURL, nil)
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET forge promos: status %d", resp.StatusCode)
	}

	var promos forgePromos
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&promos); err != nil {
		return "", err
	}

	key := op.MinecraftVersion + "-latest"
	version, ok := promos.Promos[key]
	if !ok || version == "" {
		return "", fmt.Errorf("no forge version found for mc %s", op.MinecraftVersion)
	}
	return version, nil
}
