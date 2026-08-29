package paperdl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
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
	ExpectedSHA256   string `json:"expectedSha256"`
}

var projectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
var versionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$`)

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
	if !projectPattern.MatchString(op.Project) {
		return nil, fmt.Errorf("paperDl: invalid project")
	}
	if op.MinecraftVersion != "" && op.MinecraftVersion != "latest" && !versionPattern.MatchString(op.MinecraftVersion) {
		return nil, fmt.Errorf("paperDl: invalid minecraftVersion")
	}
	if decoded, err := hex.DecodeString(op.ExpectedSHA256); err != nil || len(decoded) != sha256.Size {
		return nil, fmt.Errorf("paperDl: expectedSha256 is required")
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
	Build     int            `json:"build"`
	Downloads paperDownloads `json:"downloads"`
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
	dest, err := operations.ResolvePath(serverDir, op.Filename)
	if err != nil {
		return err
	}
	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	if err := operations.DownloadVerified(ctx, dlURL, dest, op.ExpectedSHA256, 2<<30, 10*time.Minute); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if sha == "" || !strings.EqualFold(sha, op.ExpectedSHA256) {
		return errors.New("publisher API checksum does not match the independently supplied checksum")
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
	if op.Build != "" && op.Build != "latest" {
		requested, err := strconv.Atoi(op.Build)
		if err != nil || requested <= 0 {
			return 0, "", "", errors.New("build must be a positive integer or latest")
		}
		found := false
		for _, candidate := range resp.Builds {
			if candidate.Build == requested {
				buildInfo = candidate
				found = true
				break
			}
		}
		if !found {
			return 0, "", "", fmt.Errorf("build %d is unavailable", requested)
		}
	}
	return buildInfo.Build, buildInfo.Downloads.Application.Name, buildInfo.Downloads.Application.SHA256, nil
}

func (op *PaperDl) getJSON(ctx context.Context, url string, target interface{}) error {
	client := operations.SecureHTTPClient(30 * time.Second)
	resp, err := operations.DoWithRetry(ctx, client, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %q: status %d", url, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target)
}
