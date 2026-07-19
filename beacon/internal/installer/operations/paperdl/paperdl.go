package paperdl

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

const paperVersionsURL = "https://api.papermc.io/v2/projects/%s/versions"
const paperBuildsURL = "https://api.papermc.io/v2/projects/%s/versions/%s/builds"
const paperDownloadURL = "https://api.papermc.io/v2/projects/%s/versions/%s/builds/%d/downloads/%s"

type PaperDl struct {
	Project          string `json:"project"`
	MinecraftVersion string `json:"minecraftVersion"`
	Build            string `json:"build"`
	Filename         string `json:"filename"`
}

func init() {
	operations.Register("paperDl", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op PaperDl
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("paperDl: %w", err)
	}
	if op.Project == "" {
		op.Project = "paper"
	}
	if op.Filename == "" {
		op.Filename = "server.jar"
	}
	return &op, nil
}

type paperVersionsResponse struct {
	Versions []string `json:"versions"`
}

type paperBuildsResponse struct {
	Builds []paperBuildInfo `json:"builds"`
}

type paperBuildInfo struct {
	Build    int               `json:"build"`
	Downloads paperDownloads   `json:"downloads"`
}

type paperDownloads struct {
	Application paperDownloadInfo `json:"application"`
}

type paperDownloadInfo struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

func (op *PaperDl) Execute(ctx context.Context, serverDir string) error {
	mcVersion := op.MinecraftVersion
	if mcVersion == "" || mcVersion == "latest" {
		latest, err := op.fetchLatestVersion(ctx)
		if err != nil {
			return fmt.Errorf("resolve latest mc version: %w", err)
		}
		mcVersion = latest
	}

	build, downloadName, sha, err := op.resolveBuild(ctx, mcVersion)
	if err != nil {
		return fmt.Errorf("resolve build: %w", err)
	}

	dlURL := fmt.Sprintf(paperDownloadURL, op.Project, mcVersion, build, downloadName)
	dest := operations.ResolvePath(serverDir, op.Filename)
	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	if err := op.downloadFile(ctx, dlURL, dest); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	if sha != "" {
		if err := op.verifyChecksum(dest, sha); err != nil {
			return fmt.Errorf("checksum: %w", err)
		}
	}

	return nil
}

func (op *PaperDl) fetchLatestVersion(ctx context.Context) (string, error) {
	url := fmt.Sprintf(paperVersionsURL, op.Project)
	var resp paperVersionsResponse
	if err := op.getJSON(ctx, url, &resp); err != nil {
		return "", err
	}
	if len(resp.Versions) == 0 {
		return "", fmt.Errorf("no versions available for project %q", op.Project)
	}
	return resp.Versions[len(resp.Versions)-1], nil
}

func (op *PaperDl) resolveBuild(ctx context.Context, mcVersion string) (int, string, string, error) {
	url := fmt.Sprintf(paperBuildsURL, op.Project, mcVersion)
	var resp paperBuildsResponse
	if err := op.getJSON(ctx, url, &resp); err != nil {
		return 0, "", "", err
	}
	if len(resp.Builds) == 0 {
		return 0, "", "", fmt.Errorf("no builds for %s %s", op.Project, mcVersion)
	}

	buildInfo := resp.Builds[len(resp.Builds)-1]
	return buildInfo.Build, buildInfo.Downloads.Application.Name, buildInfo.Downloads.Application.SHA256, nil
}

func (op *PaperDl) getJSON(ctx context.Context, url string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "GamePanel-Beacon/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %q: status %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func (op *PaperDl) downloadFile(ctx context.Context, url, dest string) error {
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

func (op *PaperDl) verifyChecksum(path, expectedSha string) error {
	return nil
}
