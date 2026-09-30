package mojangdl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gamepanel/beacon/internal/installer/operations"
)

const versionManifestURL = "https://launchermeta.mojang.com/mc/game/version_manifest.json"
const versionManifestV2URL = "https://launchermeta.mojang.com/mc/game/version_manifest_v2.json"

type MojangDl struct {
	Version        string `json:"version"`
	Target         string `json:"target"`
	Dest           string `json:"dest"`
	ExpectedSHA256 string `json:"expectedSha256"`
}

func init() {
	operations.Register("mojangdl", factory)
	operations.Register("mojangDl", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op MojangDl
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("mojangdl: %w", err)
	}
	// Accept both "target" (puffer) and "dest" (compat), also "filename"
	if op.Target == "" {
		op.Target = op.Dest
	}
	if op.Target == "" {
		return nil, fmt.Errorf("mojangdl: target/dest is required")
	}
	if op.Version == "" {
		op.Version = "latest"
	}
	// expectedSha256 is optional for mojangdl (Mojang manifest provides sha1); if provided must be valid.
	if op.ExpectedSHA256 != "" {
		decoded, err := hex.DecodeString(strings.TrimSpace(op.ExpectedSHA256))
		if err != nil || len(decoded) != sha256.Size {
			return nil, fmt.Errorf("mojangdl: expectedSha256 must be 64-char hex when provided")
		}
	}
	return &op, nil
}

type launcherManifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"versions"`
}

type versionDetails struct {
	Downloads map[string]struct {
		URL  string `json:"url"`
		SHA1 string `json:"sha1"`
		Size int64  `json:"size"`
	} `json:"downloads"`
}

func (op *MojangDl) Execute(ctx context.Context, serverDir string) error {
	dest, err := operations.ResolvePath(serverDir, op.Target)
	if err != nil {
		return err
	}
	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	manifestURL := versionManifestV2URL
	manifest, err := fetchManifest(ctx, manifestURL)
	if err != nil {
		// fallback to v1
		manifest, err = fetchManifest(ctx, versionManifestURL)
		if err != nil {
			return fmt.Errorf("fetch version manifest: %w", err)
		}
	}

	targetVersion := op.Version
	switch strings.ToLower(targetVersion) {
	case "latest", "release":
		targetVersion = manifest.Latest.Release
	case "snapshot":
		targetVersion = manifest.Latest.Snapshot
	}

	var versionURL string
	for _, v := range manifest.Versions {
		if v.ID == targetVersion {
			versionURL = v.URL
			break
		}
	}
	if versionURL == "" {
		return fmt.Errorf("mojang version %q not found in manifest", targetVersion)
	}

	details, err := fetchVersionDetails(ctx, versionURL)
	if err != nil {
		return fmt.Errorf("fetch version %q details: %w", targetVersion, err)
	}
	serverDl, ok := details.Downloads["server"]
	if !ok || serverDl.URL == "" {
		return fmt.Errorf("version %q has no server download", targetVersion)
	}

	// Prefer SHA256 verification if caller supplied expectedSha256; otherwise download with sha1-aware fallback via DownloadVerified not possible.
	// We use DownloadVerified when expectedSha256 provided; otherwise use SecureHTTPClient directly and stream without hash (best-effort).
	if strings.TrimSpace(op.ExpectedSHA256) != "" {
		return operations.DownloadVerified(ctx, serverDl.URL, dest, strings.TrimSpace(op.ExpectedSHA256), 2<<30, 10*time.Minute)
	}
	// No expectedSha256: download without verification but still via secure client.
	return downloadWithoutChecksum(ctx, serverDl.URL, dest)
}

func fetchManifest(ctx context.Context, url string) (*launcherManifest, error) {
	client := operations.SecureHTTPClient(30 * time.Second)
	resp, err := operations.DoWithRetry(ctx, client, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %q: status %d", url, resp.StatusCode)
	}
	var m launcherManifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func fetchVersionDetails(ctx context.Context, url string) (*versionDetails, error) {
	client := operations.SecureHTTPClient(30 * time.Second)
	resp, err := operations.DoWithRetry(ctx, client, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %q: status %d", url, resp.StatusCode)
	}
	var d versionDetails
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

func downloadWithoutChecksum(ctx context.Context, rawURL, dest string) error {
	client := operations.SecureHTTPClient(10 * time.Minute)
	resp, err := operations.DoWithRetry(ctx, client, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	})
	if err != nil {
		return fmt.Errorf("get %q: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %q: unexpected status %d", rawURL, resp.StatusCode)
	}
	const maxBytes = 2 << 30
	if resp.ContentLength > maxBytes {
		return fmt.Errorf("download exceeds %d-byte limit", maxBytes)
	}
	requiredSpace := int64(maxBytes)
	if resp.ContentLength > 0 {
		requiredSpace = resp.ContentLength
	}
	if err := operations.EnsureDiskSpace(filepath.Dir(dest), requiredSpace); err != nil {
		return err
	}
	if err := operations.EnsureParentDir(dest); err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".download-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		return err
	}
	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write %q: %w", dest, err)
	}
	if written == 0 || written > maxBytes {
		_ = tmpFile.Close()
		return fmt.Errorf("downloaded file %q is empty or too large", dest)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return err
	}
	return operations.SyncDirectory(filepath.Dir(dest))
}
