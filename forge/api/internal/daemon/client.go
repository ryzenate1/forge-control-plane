package daemon

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxRetries        = 3
	baseBackoff       = 500 * time.Millisecond
	maxBackoff        = 30 * time.Second
	retryableStatuses = "429,502,503,504"
)

var ErrMissingNodeToken = errors.New("daemon node token is required")

type commandIDContextKey struct{}

func ContextWithCommandID(ctx context.Context, commandID string) context.Context {
	return context.WithValue(ctx, commandIDContextKey{}, strings.TrimSpace(commandID))
}

func commandIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(commandIDContextKey{}).(string)
	return value
}

// isRetryableStatus returns true for status codes where a retry may succeed.
func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// jitter adds random jitter to a duration (±25%).
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	delta := d / 4
	n, err := rand.Int(rand.Reader, big.NewInt(int64(delta*2+1)))
	if err != nil {
		return d
	}
	return d - delta + time.Duration(n.Int64())
}

type Client struct {
	httpClient               *http.Client
	developmentFallbackToken string
	defaultBaseURL           string
	defaultNodeToken         string
}

func (c *Client) SetDefaultNode(baseURL, nodeToken string) {
	c.defaultBaseURL = strings.TrimRight(baseURL, "/")
	c.defaultNodeToken = nodeToken
}

func (c *Client) Get(ctx context.Context, endpoint string, dest interface{}) error {
	url := c.defaultBaseURL + endpoint
	req, err := c.newRequest(ctx, c.defaultNodeToken, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon GET %s failed with status %d: %s", endpoint, res.StatusCode, tryReadBody(res.Body))
	}
	return json.NewDecoder(res.Body).Decode(dest)
}

func (c *Client) Post(ctx context.Context, endpoint string, body, dest interface{}) error {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	url := c.defaultBaseURL + endpoint
	req, err := c.newRequest(ctx, c.defaultNodeToken, http.MethodPost, url, bodyBytes)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon POST %s failed with status %d: %s", endpoint, res.StatusCode, tryReadBody(res.Body))
	}
	if dest != nil {
		return json.NewDecoder(res.Body).Decode(dest)
	}
	return nil
}

func newRetryRoundTripper(base http.RoundTripper) http.RoundTripper {
	return &retryRoundTripper{base: base}
}

type retryRoundTripper struct {
	base http.RoundTripper
}

func (r *retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var (
		resp      *http.Response
		err       error
		bodyBytes []byte
	)
	if req.Body != nil {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := jitter(baseBackoff * (1 << (attempt - 1)))
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(backoff):
			}
			// Recreate body for retry.
			if bodyBytes != nil {
				req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
		}
		resp, err = r.base.RoundTrip(req)
		if err != nil {
			continue
		}
		if !isRetryableStatus(resp.StatusCode) {
			return resp, nil
		}
		resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	lastStatus := resp.StatusCode
	resp.Body.Close()
	return nil, fmt.Errorf("request failed after %d retries, last status: %d", maxRetries+1, lastStatus)
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{
		Timeout:   15 * time.Minute,
		Transport: newRetryRoundTripper(http.DefaultTransport),
	}}
}

// NewClientWithDevelopmentFallback preserves local Phase 0 operation for
// targets created without a stored credential. Production must use NewClient.
func NewClientWithDevelopmentFallback(nodeToken string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout:   15 * time.Minute,
			Transport: newRetryRoundTripper(http.DefaultTransport),
		},
		developmentFallbackToken: strings.TrimSpace(nodeToken),
	}
}

type PowerResponse struct {
	ServerID    string `json:"serverId"`
	Signal      string `json:"signal"`
	Accepted    bool   `json:"accepted"`
	Mode        string `json:"mode,omitempty"`
	OperationID string `json:"operationId,omitempty"`
}

type CreateRequest struct {
	ServerID        string        `json:"serverId"`
	Image           string        `json:"image"`
	Command         []string      `json:"command"`
	Env             []string      `json:"env"`
	Ports           []Port        `json:"ports"`
	Mounts          []Mount       `json:"mounts"`
	MemoryMB        int64         `json:"memoryMb"`
	SwapMB          int64         `json:"swapMb"`
	CPUShares       int64         `json:"cpuShares"`
	CPUPercent      int64         `json:"cpuPercent"`
	CPUSet          string        `json:"cpuSet,omitempty"`
	IOWeight        int64         `json:"ioWeight"`
	OOMKillDisabled bool          `json:"oomKillDisabled"`
	PIDLimit        int64         `json:"pidLimit"`
	StopSignal      string        `json:"stopSignal,omitempty"`
	StopTimeout     int64         `json:"stopTimeoutSeconds,omitempty"`
	UID             int           `json:"uid"`
	GID             int           `json:"gid"`
	DNS             []string      `json:"dns,omitempty"`
	NetworkName     string        `json:"networkName"`
	NetworkSubnet   string        `json:"networkSubnet,omitempty"`
	NetworkGateway  string        `json:"networkGateway,omitempty"`
	NetworkIP       string        `json:"networkIp,omitempty"`
	RegistryAuth    *RegistryAuth `json:"registryAuth,omitempty"`
	DiskMB          int64         `json:"diskMb"`
	Provider        string        `json:"provider,omitempty"`
}

type RegistryAuth struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	IdentityToken string `json:"identityToken,omitempty"`
	RegistryToken string `json:"registryToken,omitempty"`
	ServerAddress string `json:"serverAddress,omitempty"`
}

type Port struct {
	HostIP        string `json:"hostIp"`
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type CreateResponse struct {
	ServerID string `json:"serverId"`
	Accepted bool   `json:"accepted"`
	Mode     string `json:"mode,omitempty"`
}

type ServerConfiguration struct {
	UUID        string            `json:"uuid"`
	Name        string            `json:"name"`
	Suspended   bool              `json:"suspended"`
	Environment map[string]string `json:"environment"`
	Invocation  string            `json:"invocation"`
	DockerImage string            `json:"dockerImage"`
	Egg         map[string]any    `json:"egg"`
	Build       map[string]any    `json:"build"`
	Allocations map[string]any    `json:"allocations"`
	Config      map[string]any    `json:"config"`
	Mounts      []Mount           `json:"mounts"`
	Provider    string            `json:"provider,omitempty"`
}

type Mount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

type MountCleanupRequest struct {
	Source string `json:"source"`
}

type MountCleanupResponse struct {
	OK      bool   `json:"ok"`
	Removed bool   `json:"removed"`
	Reason  string `json:"reason,omitempty"`
}

type InstallRequest struct {
	ServerID   string            `json:"serverId"`
	Image      string            `json:"image"`
	Entrypoint string            `json:"entrypoint"`
	Script     string            `json:"script"`
	Env        map[string]string `json:"env"`
}

type InstallResponse struct {
	ServerID string `json:"serverId"`
	Accepted bool   `json:"accepted"`
	Mode     string `json:"mode,omitempty"`
	ExitCode int    `json:"exitCode"`
	Logs     string `json:"logs,omitempty"`
}

type StatsResponse struct {
	CPUPercent     float64 `json:"cpuPercent"`
	MemoryBytes    uint64  `json:"memoryBytes"`
	MemoryLimit    uint64  `json:"memoryLimit"`
	NetworkRxBytes uint64  `json:"networkRxBytes"`
	NetworkTxBytes uint64  `json:"networkTxBytes"`
}

type FileDownload struct {
	Body io.ReadCloser
	Size int64
}

type FileEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Size      int64  `json:"size"`
	ModTime   string `json:"modTime"`
}

const TransferProtocolVersion = "forge-beacon-transfer/v1"

const (
	TransferDirectionSourceControl     = "source-control"
	TransferDirectionDestinationUpload = "destination-upload"
)

type TransferCredentialClaims struct {
	Version      string    `json:"version"`
	MigrationID  string    `json:"migrationId"`
	ServerID     string    `json:"serverId"`
	SourceNodeID string    `json:"sourceNodeId"`
	TargetNodeID string    `json:"targetNodeId"`
	Direction    string    `json:"direction"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type TransferCredentialRegistration struct {
	Claims         TransferCredentialClaims `json:"claims"`
	CredentialHash string                   `json:"credentialHash"`
}

type TransferMetadata struct {
	Version      string `json:"version"`
	MigrationID  string `json:"migrationId"`
	ServerID     string `json:"serverId"`
	SourceNodeID string `json:"sourceNodeId"`
	TargetNodeID string `json:"targetNodeId"`
	Direction    string `json:"direction"`
	Phase        string `json:"phase"`
	ArchiveSize  int64  `json:"archiveSize"`
	Offset       int64  `json:"offset"`
	Checksum     string `json:"checksum"`
	Error        string `json:"error"`
}

type TransferPushRequest struct {
	DestinationURL        string `json:"destinationUrl"`
	DestinationCredential string `json:"destinationCredential"`
	IdempotencyKey        string `json:"idempotencyKey"`
}

type BackupEntry struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Checksum  string `json:"checksum"`
	Size      int64  `json:"size"`
	Status    string `json:"status"`
	Created   string `json:"created"`
	Completed string `json:"completedAt"`
}

func (c *Client) RegisterTransferCredential(ctx context.Context, baseURL, nodeToken string, registration TransferCredentialRegistration) error {
	body, err := json.Marshal(registration)
	if err != nil {
		return err
	}
	request, err := c.newRequest(ctx, nodeToken, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/transfers/credentials", body)
	if err != nil {
		return err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return daemonResponseError("register transfer credential", response)
	}
	return nil
}

func (c *Client) PrepareTransferSource(ctx context.Context, baseURL, migrationID, credential string) (TransferMetadata, error) {
	return c.transferJSON(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/transfers/"+migrationID+"/source/prepare", credential, nil)
}

func (c *Client) PushTransferSource(ctx context.Context, baseURL, migrationID, credential string, push TransferPushRequest) (TransferMetadata, error) {
	body, err := json.Marshal(push)
	if err != nil {
		return TransferMetadata{}, err
	}
	return c.transferJSON(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/transfers/"+migrationID+"/source/push", credential, body)
}

func (c *Client) RestoreTransferDestination(ctx context.Context, baseURL, migrationID, credential string) (TransferMetadata, error) {
	return c.transferJSON(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/transfers/"+migrationID+"/destination/restore", credential, nil)
}

func (c *Client) FinalizeTransferDestination(ctx context.Context, baseURL, migrationID, credential string) error {
	_, err := c.transferJSON(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/transfers/"+migrationID+"/destination/finalize", credential, nil)
	return err
}

func (c *Client) CleanupTransferSource(ctx context.Context, baseURL, migrationID, credential string) error {
	_, err := c.transferJSON(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/transfers/"+migrationID+"/source/cleanup", credential, nil)
	return err
}

func (c *Client) CancelTransfer(ctx context.Context, baseURL, migrationID, credential string) error {
	_, err := c.transferJSON(ctx, http.MethodDelete, strings.TrimRight(baseURL, "/")+"/api/v1/transfers/"+migrationID, credential, nil)
	return err
}

func (c *Client) transferJSON(ctx context.Context, method, endpoint, credential string, body []byte) (TransferMetadata, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return TransferMetadata{}, err
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return TransferMetadata{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return TransferMetadata{}, daemonResponseError("transfer request", response)
	}
	var metadata TransferMetadata
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		return TransferMetadata{}, err
	}
	return metadata, nil
}

type ResponseError struct {
	Operation  string
	StatusCode int
	Details    string
}

func (e *ResponseError) Error() string {
	message := fmt.Sprintf("daemon %s failed with status %d", e.Operation, e.StatusCode)
	if e.Details != "" {
		message += ": " + e.Details
	}
	return message
}

func daemonResponseError(operation string, response *http.Response) error {
	details := ""
	if body, err := io.ReadAll(io.LimitReader(response.Body, 16*1024)); err == nil {
		details = strings.TrimSpace(string(body))
	}
	return &ResponseError{Operation: operation, StatusCode: response.StatusCode, Details: details}
}

func (c *Client) SyncServerConfiguration(ctx context.Context, baseURL, nodeToken, serverID string, config ServerConfiguration) error {
	body, err := json.Marshal(config)
	if err != nil {
		return err
	}
	url := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/configuration"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPut, url, body)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := fmt.Sprintf("daemon config sync failed with status %d", res.StatusCode)
		if details, readErr := io.ReadAll(io.LimitReader(res.Body, 4096)); readErr == nil {
			if text := strings.TrimSpace(string(details)); text != "" {
				message += ": " + text
			}
		}
		return errors.New(message)
	}
	return nil
}

func (c *Client) InstallServer(ctx context.Context, baseURL, nodeToken, serverID string, reqBody InstallRequest) (InstallResponse, error) {
	return c.runInstaller(ctx, baseURL, nodeToken, serverID, "install", reqBody)
}

func (c *Client) ReinstallServer(ctx context.Context, baseURL, nodeToken, serverID string, reqBody InstallRequest) (InstallResponse, error) {
	return c.runInstaller(ctx, baseURL, nodeToken, serverID, "reinstall", reqBody)
}

func (c *Client) runInstaller(ctx context.Context, baseURL, nodeToken, serverID, action string, reqBody InstallRequest) (InstallResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return InstallResponse{}, err
	}
	url := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/" + action
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return InstallResponse{}, err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return InstallResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := fmt.Sprintf("daemon %s failed with status %d", action, res.StatusCode)
		if details, readErr := io.ReadAll(io.LimitReader(res.Body, 4096)); readErr == nil {
			if text := strings.TrimSpace(string(details)); text != "" {
				message += ": " + text
			}
		}
		return InstallResponse{}, errors.New(message)
	}
	var payload InstallResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return InstallResponse{}, err
	}
	return payload, nil
}

func (c *Client) CreateServer(ctx context.Context, baseURL, nodeToken string, reqBody CreateRequest) (CreateResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return CreateResponse{}, err
	}

	url := strings.TrimRight(baseURL, "/") + "/servers"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return CreateResponse{}, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return CreateResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := fmt.Sprintf("daemon create request failed with status %d", res.StatusCode)
		if details, readErr := io.ReadAll(io.LimitReader(res.Body, 4096)); readErr == nil {
			text := strings.TrimSpace(string(details))
			if text != "" {
				message += ": " + text
			}
		}
		return CreateResponse{}, errors.New(message)
	}

	var payload CreateResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return CreateResponse{}, err
	}
	return payload, nil
}

func (c *Client) Logs(ctx context.Context, baseURL, nodeToken, serverID string) (string, error) {
	url := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/logs"
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("daemon logs request failed with status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 256*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) SendCommand(ctx context.Context, baseURL, nodeToken, serverID, command string) error {
	body, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		return err
	}
	url := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/command"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon command request failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) SendPower(ctx context.Context, baseURL, nodeToken, serverID, signal string) (PowerResponse, error) {
	body, err := json.Marshal(map[string]string{"signal": signal})
	if err != nil {
		return PowerResponse{}, err
	}
	url := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/power"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return PowerResponse{}, err
	}
	if commandID := commandIDFromContext(ctx); commandID != "" {
		req.Header.Set("X-Forge-Command-ID", commandID)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return PowerResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return PowerResponse{}, fmt.Errorf("daemon power request failed with status %d", res.StatusCode)
	}

	var payload PowerResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return PowerResponse{}, err
	}
	return payload, nil
}

func (c *Client) DeleteServer(ctx context.Context, baseURL, nodeToken, serverID string) (PowerResponse, error) {
	url := strings.TrimRight(baseURL, "/") + "/servers/" + serverID
	req, err := c.newRequest(ctx, nodeToken, http.MethodDelete, url, nil)
	if err != nil {
		return PowerResponse{}, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return PowerResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return PowerResponse{}, fmt.Errorf("daemon delete request failed with status %d", res.StatusCode)
	}

	var payload PowerResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return PowerResponse{}, err
	}
	return payload, nil
}

func (c *Client) Stats(ctx context.Context, baseURL, nodeToken, serverID string) (StatsResponse, error) {
	url := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/stats"
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, url, nil)
	if err != nil {
		return StatsResponse{}, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return StatsResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return StatsResponse{}, fmt.Errorf("daemon stats request failed with status %d", res.StatusCode)
	}
	var payload StatsResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return StatsResponse{}, err
	}
	return payload, nil
}

func (c *Client) CreateBackup(ctx context.Context, baseURL, nodeToken, serverID string, ignoredFiles []string) (BackupEntry, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/backups"
	var body []byte
	if len(ignoredFiles) > 0 {
		var reqBody struct {
			IgnoredFiles []string `json:"ignored_files"`
		}
		reqBody.IgnoredFiles = ignoredFiles
		var err error
		body, err = json.Marshal(reqBody)
		if err != nil {
			return BackupEntry{}, fmt.Errorf("marshal ignored_files: %w", err)
		}
	}
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, body)
	if err != nil {
		return BackupEntry{}, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return BackupEntry{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return BackupEntry{}, fmt.Errorf("daemon backup create request failed with status %d", res.StatusCode)
	}
	var payload BackupEntry
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return BackupEntry{}, err
	}
	return payload, nil
}

func (c *Client) ListBackups(ctx context.Context, baseURL, nodeToken, serverID string) ([]BackupEntry, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/backups"
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("daemon backup list request failed with status %d", res.StatusCode)
	}
	var payload []BackupEntry
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) DownloadBackup(ctx context.Context, baseURL, nodeToken, serverID, name string) (io.ReadCloser, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/backups/download?name=" + url.QueryEscape(name)
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/zip")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_ = res.Body.Close()
		return nil, fmt.Errorf("daemon backup download request failed with status %d", res.StatusCode)
	}
	return res.Body, nil
}

func (c *Client) RestoreBackup(ctx context.Context, baseURL, nodeToken, serverID, name string, truncate bool) error {
	body, err := json.Marshal(map[string]interface{}{"name": name, "truncate": truncate})
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/backups/restore"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon backup restore request failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) DeleteBackup(ctx context.Context, baseURL, nodeToken, serverID, name string) error {
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/backups?name=" + url.QueryEscape(name)
	req, err := c.newRequest(ctx, nodeToken, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon backup delete request failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) ArchiveFiles(ctx context.Context, baseURL, nodeToken, serverID, path string) (io.ReadCloser, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/files/archive?path=" + url.QueryEscape(path)
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/gzip")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_ = res.Body.Close()
		return nil, fmt.Errorf("daemon archive request failed with status %d", res.StatusCode)
	}
	return res.Body, nil
}

func (c *Client) DecompressFile(ctx context.Context, baseURL, nodeToken, serverID, path string) error {
	body, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/files/decompress"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon decompress request failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) ListFiles(ctx context.Context, baseURL, nodeToken, serverID, path string) ([]FileEntry, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/files?path=" + url.QueryEscape(path)
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("daemon file list request failed with status %d", res.StatusCode)
	}
	var payload []FileEntry
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) DownloadFile(ctx context.Context, baseURL, nodeToken, serverID, path string) (FileDownload, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/files/download?path=" + url.QueryEscape(path)
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, endpoint, nil)
	if err != nil {
		return FileDownload{}, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return FileDownload{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		return FileDownload{}, daemonResponseError("file download", res)
	}
	return FileDownload{Body: res.Body, Size: res.ContentLength}, nil
}

func (c *Client) ReadFile(ctx context.Context, baseURL, nodeToken, serverID, path string) (string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/files/content?path=" + url.QueryEscape(path)
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("daemon file read request failed with status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1024*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) PullRemoteFile(ctx context.Context, baseURL, nodeToken, serverID, sourceURL, target, fileName string) error {
	body, err := json.Marshal(map[string]string{"url": sourceURL, "target": target, "fileName": fileName})
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/files/pull"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return daemonResponseError("remote file pull", res)
	}
	return nil
}

func (c *Client) WriteFile(ctx context.Context, baseURL, nodeToken, serverID, path string, body []byte) error {
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/files/content?path=" + url.QueryEscape(path)
	req, err := c.newRequest(ctx, nodeToken, http.MethodPut, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon file write request failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) UploadFileChunk(ctx context.Context, baseURL, nodeToken, serverID, path, uploadID string, offset int64, final bool, body io.Reader) error {
	endpoint := strings.TrimRight(baseURL, "/") +
		"/servers/" + serverID +
		"/files/upload?path=" + url.QueryEscape(path) +
		"&uploadId=" + url.QueryEscape(uploadID) +
		"&offset=" + url.QueryEscape(fmt.Sprintf("%d", offset)) +
		"&final=" + url.QueryEscape(fmt.Sprintf("%t", final))
	req, err := c.newStreamRequest(ctx, nodeToken, http.MethodPut, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon file upload request failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) CopyFile(ctx context.Context, baseURL, nodeToken, serverID, from, to string) error {
	return c.fileJSONMutation(ctx, baseURL, nodeToken, serverID, "copy", map[string]string{"from": from, "to": to})
}

func (c *Client) ChmodFile(ctx context.Context, baseURL, nodeToken, serverID, path, mode string) error {
	return c.fileJSONMutation(ctx, baseURL, nodeToken, serverID, "chmod", map[string]string{"path": path, "mode": mode})
}

func (c *Client) DeleteFiles(ctx context.Context, baseURL, nodeToken, serverID string, paths []string) error {
	return c.fileJSONPayloadMutation(ctx, baseURL, nodeToken, serverID, "delete-batch", map[string]any{"paths": paths})
}

func (c *Client) RenameFiles(ctx context.Context, baseURL, nodeToken, serverID string, files []map[string]string) error {
	return c.fileJSONPayloadMutation(ctx, baseURL, nodeToken, serverID, "rename-batch", map[string]any{"files": files})
}

func (c *Client) fileJSONMutation(ctx context.Context, baseURL, nodeToken, serverID, operation string, payload map[string]string) error {
	return c.fileJSONPayloadMutation(ctx, baseURL, nodeToken, serverID, operation, payload)
}

func (c *Client) fileJSONPayloadMutation(ctx context.Context, baseURL, nodeToken, serverID, operation string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/files/" + operation
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return daemonResponseError("file "+operation, res)
	}
	return nil
}

func (c *Client) DeleteFile(ctx context.Context, baseURL, nodeToken, serverID, path string) error {
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/files?path=" + url.QueryEscape(path)
	req, err := c.newRequest(ctx, nodeToken, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon file delete request failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) MakeDir(ctx context.Context, baseURL, nodeToken, serverID, path string) error {
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/files/mkdir?path=" + url.QueryEscape(path)
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon mkdir request failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) RenameFile(ctx context.Context, baseURL, nodeToken, serverID, from, to string) error {
	body, err := json.Marshal(map[string]string{"from": from, "to": to})
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/files/rename"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPatch, endpoint, body)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon rename request failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) WebSocketURL(baseURL, serverID, stream string) (string, string) {
	path := "/servers/" + serverID + "/ws/" + stream
	endpoint := strings.TrimRight(baseURL, "/") + path
	endpoint = strings.TrimPrefix(endpoint, "http://")
	if endpoint != strings.TrimRight(baseURL, "/")+path {
		return "ws://" + endpoint, path
	}
	endpoint = strings.TrimPrefix(strings.TrimRight(baseURL, "/")+path, "https://")
	return "wss://" + endpoint, path
}

func (c *Client) SignedHeaders(nodeToken, method, requestURI string, body []byte) (http.Header, error) {
	nodeToken, err := c.resolveNodeToken(nodeToken)
	if err != nil {
		return nil, err
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	headers := http.Header{}
	headers.Set("X-Panel-Timestamp", timestamp)
	headers.Set("X-Panel-Signature", sign(nodeToken, method, requestURI, timestamp, body))
	return headers, nil
}

// HTTPClient returns the underlying HTTP client for direct request forwarding.
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

func (c *Client) HostFilesList(ctx context.Context, baseURL, nodeToken, path string) (json.RawMessage, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/files/list?path=" + url.QueryEscape(path)
	return c.hostGetJSON(ctx, nodeToken, endpoint)
}

func (c *Client) HostFilesRead(ctx context.Context, baseURL, nodeToken, path string) (string, error) {
	body, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/files/read"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("host file read failed with status %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 10*1024*1024))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *Client) HostFilesWrite(ctx context.Context, baseURL, nodeToken, path, content string) error {
	payload, err := json.Marshal(map[string]string{"path": path, "content": content})
	if err != nil {
		return err
	}
	return c.hostPost(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/files/write", payload)
}

func (c *Client) HostFilesMkdir(ctx context.Context, baseURL, nodeToken, path string) error {
	payload, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		return err
	}
	return c.hostPost(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/files/mkdir", payload)
}

func (c *Client) HostFilesRemove(ctx context.Context, baseURL, nodeToken, path string) error {
	payload, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		return err
	}
	return c.hostPost(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/files/remove", payload)
}

func (c *Client) HostFilesRename(ctx context.Context, baseURL, nodeToken, oldPath, newPath string) error {
	payload, err := json.Marshal(map[string]string{"source": oldPath, "target": newPath})
	if err != nil {
		return err
	}
	return c.hostPost(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/files/rename", payload)
}

func (c *Client) HostFilesCopy(ctx context.Context, baseURL, nodeToken, sourcePath, destPath string) error {
	payload, err := json.Marshal(map[string]string{"source": sourcePath, "target": destPath})
	if err != nil {
		return err
	}
	return c.hostPost(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/files/copy", payload)
}

func (c *Client) HostFilesChmod(ctx context.Context, baseURL, nodeToken, path, mode string) error {
	payload, err := json.Marshal(map[string]string{"path": path, "mode": mode})
	if err != nil {
		return err
	}
	return c.hostPost(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/files/chmod", payload)
}

func (c *Client) HostFilesUpload(ctx context.Context, baseURL, nodeToken, path string, body io.Reader, contentType string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/v1/files/upload?path="+url.QueryEscape(path), body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	headers, err := c.SignedHeaders(nodeToken, req.Method, req.URL.RequestURI(), nil)
	if err != nil {
		return err
	}
	copyHeaders(req.Header, headers)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("host file upload failed with status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) HostFilesDownload(ctx context.Context, baseURL, nodeToken, path string) (io.ReadCloser, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/files/download?path=" + url.QueryEscape(path)
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_ = res.Body.Close()
		return nil, fmt.Errorf("host file download failed with status %d", res.StatusCode)
	}
	return res.Body, nil
}

func (c *Client) HostTerminalWSURL(baseURL string) (string, string) {
	path := "/v1/terminal/ws"
	endpoint := strings.TrimRight(baseURL, "/") + path
	endpoint = strings.TrimPrefix(endpoint, "http://")
	if endpoint != strings.TrimRight(baseURL, "/")+path {
		return "ws://" + endpoint, path
	}
	endpoint = strings.TrimPrefix(strings.TrimRight(baseURL, "/")+path, "https://")
	return "wss://" + endpoint, path
}

func (c *Client) hostGetJSON(ctx context.Context, nodeToken, url string) (json.RawMessage, error) {
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("host request failed with status %d: %s", res.StatusCode, tryReadBody(res.Body))
	}
	var payload json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) hostPost(ctx context.Context, nodeToken, url string, payload []byte) error {
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("host request failed with status %d: %s", res.StatusCode, tryReadBody(res.Body))
	}
	return nil
}

func tryReadBody(r io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(r, 4096))
	return strings.TrimSpace(string(data))
}

func (c *Client) newRequest(ctx context.Context, nodeToken, method, url string, body []byte) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	headers, err := c.SignedHeaders(nodeToken, method, req.URL.RequestURI(), body)
	if err != nil {
		return nil, err
	}
	copyHeaders(req.Header, headers)
	return req, nil
}

func (c *Client) newStreamRequest(ctx context.Context, nodeToken, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	headers, err := c.SignedHeaders(nodeToken, method, req.URL.RequestURI(), nil)
	if err != nil {
		return nil, err
	}
	copyHeaders(req.Header, headers)
	return req, nil
}

func (c *Client) resolveNodeToken(nodeToken string) (string, error) {
	if nodeToken = strings.TrimSpace(nodeToken); nodeToken != "" {
		return nodeToken, nil
	}
	if c != nil && c.developmentFallbackToken != "" {
		return c.developmentFallbackToken, nil
	}
	return "", ErrMissingNodeToken
}

func (c *Client) CleanupMount(ctx context.Context, baseURL, nodeToken, mountSource string) (MountCleanupResponse, error) {
	body, err := json.Marshal(MountCleanupRequest{Source: mountSource})
	if err != nil {
		return MountCleanupResponse{}, err
	}
	url := strings.TrimRight(baseURL, "/") + "/mounts/cleanup"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return MountCleanupResponse{}, err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return MountCleanupResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := fmt.Sprintf("daemon mount cleanup failed with status %d", res.StatusCode)
		if details, readErr := io.ReadAll(io.LimitReader(res.Body, 4096)); readErr == nil {
			if text := strings.TrimSpace(string(details)); text != "" {
				message += ": " + text
			}
		}
		return MountCleanupResponse{}, errors.New(message)
	}
	var payload MountCleanupResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return MountCleanupResponse{}, err
	}
	return payload, nil
}

func copyHeaders(destination, source http.Header) {
	for key, values := range source {
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}

func (c *Client) AdminContainerList(ctx context.Context, baseURL, nodeToken string, all bool) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/containers"
	if all {
		url += "?all=true"
	}
	return c.adminGetJSON(ctx, nodeToken, url)
}

func (c *Client) AdminContainerInspect(ctx context.Context, baseURL, nodeToken, containerID string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/containers/" + containerID
	return c.adminGetJSON(ctx, nodeToken, url)
}

func (c *Client) AdminContainerLogs(ctx context.Context, baseURL, nodeToken, containerID, tail string) (string, error) {
	path := strings.TrimRight(baseURL, "/") + "/api/admin/containers/" + containerID + "/logs"
	if tail != "" {
		path += "?tail=" + tail
	}
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("daemon container logs failed with status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 512*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) AdminContainerStart(ctx context.Context, baseURL, nodeToken, containerID string) error {
	return c.adminContainerAction(ctx, baseURL, nodeToken, containerID, "start")
}

func (c *Client) AdminContainerStop(ctx context.Context, baseURL, nodeToken, containerID string) error {
	return c.adminContainerAction(ctx, baseURL, nodeToken, containerID, "stop")
}

func (c *Client) AdminContainerRestart(ctx context.Context, baseURL, nodeToken, containerID string) error {
	return c.adminContainerAction(ctx, baseURL, nodeToken, containerID, "restart")
}

func (c *Client) adminContainerAction(ctx context.Context, baseURL, nodeToken, containerID, action string) error {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/containers/" + containerID + "/" + action
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon container %s failed with status %d", action, res.StatusCode)
	}
	return nil
}

func (c *Client) AdminContainerDelete(ctx context.Context, baseURL, nodeToken, containerID string, force, removeVolumes bool) error {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/containers/" + containerID + "?force=" + fmt.Sprintf("%t", force) + "&v=" + fmt.Sprintf("%t", removeVolumes)
	req, err := c.newRequest(ctx, nodeToken, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon container delete failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) AdminImageList(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/images"
	return c.adminGetJSON(ctx, nodeToken, url)
}

func (c *Client) AdminImagePull(ctx context.Context, baseURL, nodeToken, image, registryAuth string) error {
	body, err := json.Marshal(map[string]string{"image": image, "registryAuth": registryAuth})
	if err != nil {
		return err
	}
	url := strings.TrimRight(baseURL, "/") + "/api/admin/images/pull"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon image pull failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) AdminImageDelete(ctx context.Context, baseURL, nodeToken, imageID string, force bool) error {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/images/" + imageID + "?force=" + fmt.Sprintf("%t", force)
	req, err := c.newRequest(ctx, nodeToken, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon image delete failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) AdminImagePrune(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/images/prune"
	return c.adminPostJSON(ctx, nodeToken, url, nil)
}

func (c *Client) AdminNetworkList(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/networks"
	return c.adminGetJSON(ctx, nodeToken, url)
}

func (c *Client) AdminNetworkInspect(ctx context.Context, baseURL, nodeToken, networkID string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/networks/" + networkID
	return c.adminGetJSON(ctx, nodeToken, url)
}

func (c *Client) AdminVolumeList(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/volumes"
	return c.adminGetJSON(ctx, nodeToken, url)
}

func (c *Client) AdminVolumeInspect(ctx context.Context, baseURL, nodeToken, volumeID string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/volumes/" + volumeID
	return c.adminGetJSON(ctx, nodeToken, url)
}

func (c *Client) AdminVolumeUsage(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/volumes/usage"
	return c.adminGetJSON(ctx, nodeToken, url)
}

func (c *Client) AdminContainerPause(ctx context.Context, baseURL, nodeToken, containerID string) error {
	return c.adminContainerAction(ctx, baseURL, nodeToken, containerID, "pause")
}

func (c *Client) AdminContainerUnpause(ctx context.Context, baseURL, nodeToken, containerID string) error {
	return c.adminContainerAction(ctx, baseURL, nodeToken, containerID, "unpause")
}

func (c *Client) AdminContainerCreate(ctx context.Context, baseURL, nodeToken string, req map[string]any) (json.RawMessage, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(baseURL, "/") + "/api/admin/containers"
	return c.adminPostJSON(ctx, nodeToken, url, body)
}

func (c *Client) AdminContainerStats(ctx context.Context, baseURL, nodeToken, containerID string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/containers/" + containerID + "/stats"
	return c.adminGetJSON(ctx, nodeToken, url)
}

// --- Docker Image Build/Push/Tag/Search ---

func (c *Client) AdminImageBuild(ctx context.Context, baseURL, nodeToken, dockerfile, tag string) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]string{"dockerfile": dockerfile, "tag": tag})
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(baseURL, "/") + "/api/admin/images/build"
	return c.adminPostJSON(ctx, nodeToken, url, body)
}

func (c *Client) AdminImagePush(ctx context.Context, baseURL, nodeToken, imageRef string, auth *RegistryAuth) error {
	payload := map[string]any{"imageRef": imageRef}
	if auth != nil {
		payload["registryAuth"] = auth
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := strings.TrimRight(baseURL, "/") + "/image/push"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon image push failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) AdminImageTag(ctx context.Context, baseURL, nodeToken, imageID, repo, tag string) error {
	body, err := json.Marshal(map[string]string{"repo": repo, "tag": tag})
	if err != nil {
		return err
	}
	url := strings.TrimRight(baseURL, "/") + "/api/admin/images/" + imageID + "/tag"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon image tag failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) AdminImageSearch(ctx context.Context, baseURL, nodeToken, term string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/images/search?term=" + url.QueryEscape(term)
	return c.adminGetJSON(ctx, nodeToken, url)
}

// --- Container File Operations ---

func (c *Client) AdminContainerFilesList(ctx context.Context, baseURL, nodeToken, containerID, filePath string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/containers/" + containerID + "/files?path=" + url.QueryEscape(filePath)
	return c.adminGetJSON(ctx, nodeToken, url)
}

func (c *Client) AdminContainerFilesRead(ctx context.Context, baseURL, nodeToken, containerID, filePath string) (string, error) {
	body, err := json.Marshal(map[string]string{"path": filePath})
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(baseURL, "/") + "/api/admin/containers/" + containerID + "/files/read"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("daemon container file read failed with status %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 10*1024*1024))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *Client) AdminContainerFilesUpload(ctx context.Context, baseURL, nodeToken, containerID, destPath string, body io.Reader, contentType string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/api/admin/containers/"+containerID+"/files/upload?path="+url.QueryEscape(destPath), body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	headers, err := c.SignedHeaders(nodeToken, req.Method, req.URL.RequestURI(), nil)
	if err != nil {
		return err
	}
	copyHeaders(req.Header, headers)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("daemon container file upload failed with status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) AdminContainerFilesDelete(ctx context.Context, baseURL, nodeToken, containerID, filePath string) error {
	body, err := json.Marshal(map[string]string{"path": filePath})
	if err != nil {
		return err
	}
	url := strings.TrimRight(baseURL, "/") + "/api/admin/containers/" + containerID + "/files/delete"
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon container file delete failed with status %d", res.StatusCode)
	}
	return nil
}

// --- Admin Network ---

func (c *Client) AdminNetworkCreate(ctx context.Context, baseURL, nodeToken string, req map[string]any) (json.RawMessage, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(baseURL, "/") + "/api/admin/networks"
	return c.adminPostJSON(ctx, nodeToken, url, body)
}

func (c *Client) AdminNetworkDelete(ctx context.Context, baseURL, nodeToken, networkID string) error {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/networks/" + networkID
	req, err := c.newRequest(ctx, nodeToken, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon network delete failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) AdminVolumeCreate(ctx context.Context, baseURL, nodeToken string, req map[string]any) (json.RawMessage, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(baseURL, "/") + "/api/admin/volumes"
	return c.adminPostJSON(ctx, nodeToken, url, body)
}

func (c *Client) AdminVolumeDelete(ctx context.Context, baseURL, nodeToken, volumeName string) error {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/volumes/" + volumeName
	req, err := c.newRequest(ctx, nodeToken, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("daemon volume delete failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) AdminVolumePrune(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/volumes/prune"
	return c.adminPostJSON(ctx, nodeToken, url, nil)
}

func (c *Client) AdminContainerExec(ctx context.Context, baseURL, nodeToken, containerID string, cmd []string, tty bool) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{
		"cmd": cmd, "attachStdout": true, "attachStderr": true, "tty": tty,
	})
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(baseURL, "/") + "/api/admin/containers/" + containerID + "/exec"
	return c.adminPostJSON(ctx, nodeToken, url, body)
}

func (c *Client) AdminContainerTop(ctx context.Context, baseURL, nodeToken, containerID string) (json.RawMessage, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/admin/containers/" + containerID + "/top"
	return c.adminGetJSON(ctx, nodeToken, url)
}

func (c *Client) adminGetJSON(ctx context.Context, nodeToken, url string) (json.RawMessage, error) {
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("daemon admin request failed with status %d", res.StatusCode)
	}
	var payload json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) adminPostJSON(ctx context.Context, nodeToken, url string, body []byte) (json.RawMessage, error) {
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("daemon admin request failed with status %d", res.StatusCode)
	}
	var payload json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func sign(token, method, requestURI, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(method))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(requestURI))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
