package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type DockerfileBuildRequest struct {
	WorkspaceID    string        `json:"workspaceId"`
	SourceDir      string        `json:"sourceDir"` // deprecated: use WorkspaceID
	Dockerfile     string        `json:"dockerfile,omitempty"`
	ImageName      string        `json:"imageName"`
	BuildArgs      []string      `json:"buildArgs,omitempty"`
	Labels         []string      `json:"labels,omitempty"`
	Tags           []string      `json:"tags,omitempty"`
	NoCache        bool          `json:"noCache"`
	CacheFrom      []string      `json:"cacheFrom,omitempty"`
	CacheTo        []string      `json:"cacheTo,omitempty"`
	Platform       string        `json:"platform,omitempty"`
	RegistryAuth   *RegistryAuth `json:"registryAuth,omitempty"`
	SecretArgs     []string      `json:"secretArgs,omitempty"`
	MaxCPU         int           `json:"maxCpu,omitempty"`
	MaxMemoryMB    int           `json:"maxMemoryMb,omitempty"`
	MaxLogBytes    int           `json:"maxLogBytes,omitempty"`
	IdempotencyKey string        `json:"idempotencyKey,omitempty"`
	CredentialPatterns []string  `json:"credentialPatterns,omitempty"`
	TenantID       string        `json:"tenantId,omitempty"`
}

type NixpacksBuildRequest struct {
	WorkspaceID    string        `json:"workspaceId"`
	SourceDir      string        `json:"sourceDir"` // deprecated: use WorkspaceID
	ImageName      string        `json:"imageName"`
	BuildArgs      []string      `json:"buildArgs,omitempty"`
	Tags           []string      `json:"tags,omitempty"`
	NoCache        bool          `json:"noCache"`
	Platform       string        `json:"platform,omitempty"`
	RegistryAuth   *RegistryAuth `json:"registryAuth,omitempty"`
	SecretArgs     []string      `json:"secretArgs,omitempty"`
	MaxCPU         int           `json:"maxCpu,omitempty"`
	MaxMemoryMB    int           `json:"maxMemoryMb,omitempty"`
	MaxLogBytes    int           `json:"maxLogBytes,omitempty"`
	IdempotencyKey string        `json:"idempotencyKey,omitempty"`
	CredentialPatterns []string  `json:"credentialPatterns,omitempty"`
	TenantID       string        `json:"tenantId,omitempty"`
}

type BuildStartResponse struct {
	ID        string `json:"id"`
	ImageName string `json:"imageName"`
	Status    string `json:"status"`
}

type BuildLogLine struct {
	BuildID   string    `json:"buildId"`
	Timestamp time.Time `json:"timestamp"`
	Line      string    `json:"line"`
}

func (c *Client) DockerfileBuild(ctx context.Context, baseURL, nodeToken string, req DockerfileBuildRequest) (*BuildStartResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal dockerfile build request: %w", err)
	}
	url := strings.TrimRight(baseURL, "/") + "/build/dockerfile"
	request, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("dockerfile build request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, daemonResponseError("dockerfile build", resp)
	}
	var result BuildStartResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode build response: %w", err)
	}
	return &result, nil
}

func (c *Client) NixpacksBuild(ctx context.Context, baseURL, nodeToken string, req NixpacksBuildRequest) (*BuildStartResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal nixpacks build request: %w", err)
	}
	url := strings.TrimRight(baseURL, "/") + "/build/nixpacks"
	request, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("nixpacks build request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, daemonResponseError("nixpacks build", resp)
	}
	var result BuildStartResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode build response: %w", err)
	}
	return &result, nil
}

func (c *Client) BuildLogs(ctx context.Context, baseURL, nodeToken, buildID string, follow bool) ([]BuildLogLine, error) {
	url := fmt.Sprintf("%s/build/logs?id=%s&follow=%t", strings.TrimRight(baseURL, "/"), buildID, follow)
	request, err := c.newRequest(ctx, nodeToken, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/event-stream")
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("build logs request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, daemonResponseError("build logs", resp)
	}

	var lines []BuildLogLine
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			lines = append(lines, BuildLogLine{
				BuildID:   buildID,
				Timestamp: time.Now(),
				Line:      strings.TrimPrefix(line, "data: "),
			})
		}
	}
	return lines, scanner.Err()
}

func (c *Client) BuildLogsStream(ctx context.Context, baseURL, nodeToken, buildID string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/build/logs?id=%s&follow=true", strings.TrimRight(baseURL, "/"), buildID)
	request, err := c.newRequest(ctx, nodeToken, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/event-stream")
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("build logs stream request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, daemonResponseError("build logs stream", resp)
	}
	return resp.Body, nil
}

type BuildCancelRequest struct {
	ID string `json:"id"`
}

func (c *Client) CancelBuild(ctx context.Context, baseURL, nodeToken, buildID string) error {
	body, err := json.Marshal(BuildCancelRequest{ID: buildID})
	if err != nil {
		return fmt.Errorf("marshal cancel request: %w", err)
	}
	url := strings.TrimRight(baseURL, "/") + "/build/cancel"
	request, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("cancel build request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return daemonResponseError("cancel build", resp)
	}
	return nil
}

type GitCloneRequest struct {
	RepoURL   string `json:"repoUrl"`
	Branch    string `json:"branch"`
	SourceID  string `json:"sourceId"`
	CommitSHA string `json:"commitSha,omitempty"`
}

type GitCloneResponse struct {
	WorkspaceID string `json:"workspaceId"`
	CommitSHA   string `json:"commitSha"`
}

func (c *Client) GitClone(ctx context.Context, baseURL, nodeToken string, req GitCloneRequest) (*GitCloneResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal git clone request: %w", err)
	}
	url := strings.TrimRight(baseURL, "/") + "/git/clone"
	request, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("git clone request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, daemonResponseError("git clone", resp)
	}
	var result GitCloneResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode git clone response: %w", err)
	}
	return &result, nil
}

func (c *Client) GitCleanup(ctx context.Context, baseURL, nodeToken, workspaceID string) error {
	body, err := json.Marshal(map[string]string{"workspaceId": workspaceID})
	if err != nil {
		return fmt.Errorf("marshal git cleanup request: %w", err)
	}
	url := strings.TrimRight(baseURL, "/") + "/git/cleanup"
	request, err := c.newRequest(ctx, nodeToken, http.MethodDelete, url, body)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("git cleanup request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return daemonResponseError("git cleanup", resp)
	}
	return nil
}

type CapabilitiesResponse struct {
	NodeID       string             `json:"nodeId,omitempty"`
	Architecture string             `json:"architecture"`
	Capabilities []CapabilityEntry  `json:"capabilities"`
	BuildInfo    *BuildCapability   `json:"buildInfo,omitempty"`
}

type CapabilityEntry struct {
	Type    string `json:"type"`
	Version string `json:"version,omitempty"`
	Status  string `json:"status"`
}

type BuildCapability struct {
	DockerBuildEnabled bool `json:"dockerBuildEnabled"`
	NixpacksEnabled    bool `json:"nixpacksEnabled"`
}

func (c *Client) GetNodeCapabilities(ctx context.Context, baseURL, nodeToken string) (*CapabilitiesResponse, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/capabilities"
	request, err := c.newRequest(ctx, nodeToken, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("capabilities request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, daemonResponseError("capabilities", resp)
	}
	var result CapabilitiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode capabilities response: %w", err)
	}
	return &result, nil
}

type ImagePushRequest struct {
	ImageRef     string        `json:"imageRef"`
	RegistryAuth *RegistryAuth `json:"registryAuth,omitempty"`
}

type PushResult struct {
	Digest string `json:"digest"`
}

func (c *Client) PushImage(ctx context.Context, baseURL, nodeToken string, req ImagePushRequest) (*PushResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal push request: %w", err)
	}
	url := strings.TrimRight(baseURL, "/") + "/image/push"
	request, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("push image request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, daemonResponseError("push image", resp)
	}
	var result PushResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode push result: %w", err)
	}
	return &result, nil
}

func DigestFromBuildOutput(logs string) string {
	scanner := bufio.NewScanner(strings.NewReader(logs))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "digest:") {
			parts := strings.Split(line, "digest:")
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func maskCredentials(logs string, creds []string) string {
	masked := logs
	for _, cred := range creds {
		if cred == "" {
			continue
		}
		masked = strings.ReplaceAll(masked, cred, "****")
	}
	return masked
}

func (c *Client) InspectImageDigest(ctx context.Context, baseURL, nodeToken, imageRef string) (string, error) {
	url := fmt.Sprintf("%s/image/inspect?ref=%s", strings.TrimRight(baseURL, "/"), url.QueryEscape(imageRef))
	request, err := c.newRequest(ctx, nodeToken, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("inspect image request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", daemonResponseError("inspect image", resp)
	}
	var result struct {
		Digest string `json:"digest,omitempty"`
		ID     string `json:"id,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode inspect result: %w", err)
	}
	if result.Digest != "" {
		return result.Digest, nil
	}
	return "", fmt.Errorf("image digest not available for %s", imageRef)
}

func (c *Client) LoginRegistry(ctx context.Context, baseURL, nodeToken string, auth RegistryAuth) error {
	body, err := json.Marshal(auth)
	if err != nil {
		return fmt.Errorf("marshal login request: %w", err)
	}
	url := strings.TrimRight(baseURL, "/") + "/registry/login"
	request, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("registry login request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return daemonResponseError("registry login", resp)
	}
	return nil
}

func (c *Client) BuildCleanup(ctx context.Context, baseURL, nodeToken, workspaceID string) error {
	body, err := json.Marshal(map[string]string{"workspaceId": workspaceID})
	if err != nil {
		return fmt.Errorf("marshal cleanup request: %w", err)
	}
	url := strings.TrimRight(baseURL, "/") + "/build/cleanup"
	request, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("build cleanup request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return daemonResponseError("build cleanup", resp)
	}
	return nil
}

type BuildStatusResponse struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	ExitCode int    `json:"exitCode,omitempty"`
}

func (c *Client) GetBuildStatus(ctx context.Context, baseURL, nodeToken, buildID string) (*BuildStatusResponse, error) {
	url := fmt.Sprintf("%s/build/status?id=%s", strings.TrimRight(baseURL, "/"), buildID)
	request, err := c.newRequest(ctx, nodeToken, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("build status request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, daemonResponseError("build status", resp)
	}
	var result BuildStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode build status: %w", err)
	}
	return &result, nil
}
