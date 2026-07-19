package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxErrorBodyBytes = 16 * 1024

// Client interface for panel communication
type Client interface {
	GetServerConfiguration(ctx context.Context, uuid string) (ServerConfigurationResponse, error)
	GetServers(ctx context.Context, perPage int) ([]RawServerData, error)
	ResetServersState(ctx context.Context) error
	SendActivityLogs(ctx context.Context, activity []Activity) error
	SendServerStats(ctx context.Context, serverID string, stats ServerStats) error
	SendNodeHeartbeat(ctx context.Context, nodeID string, heartbeat NodeHeartbeat) error
	CreatePlacementReservation(ctx context.Context, req PlacementReservationRequest) (PlacementReservation, error)
	ConfirmPlacementReservation(ctx context.Context, reservationID string) error
	CancelPlacementReservation(ctx context.Context, reservationID string) error
	TriggerServerBackup(ctx context.Context, serverID string) error
	ReportEvacuationProgress(ctx context.Context, evacuationID string, progress EvacuationProgress) error
	SetInstallationStatus(ctx context.Context, serverID string, successful bool) error
	SendCrashEvent(ctx context.Context, serverID string, exitCode int, oomKilled bool, autoRestart bool) error
	SendBackupStatus(ctx context.Context, serverID string, req BackupStatusRequest) error
	SendRestoreStatus(ctx context.Context, serverID string, req RestoreStatusRequest) error
}

type client struct {
	remoteBaseURL string
	apiBaseURL    string
	token         string
	httpClient    *http.Client
}

// NewClient accepts the panel root URL (or a URL ending in /api/v1 or
// /api/remote) and derives the two API roots deliberately. Remote daemon calls
// use /api/remote, while node heartbeat uses the Forge /api/v1 route.
func NewClient(panelURL, token string) Client {
	panelURL = normalizePanelBaseURL(panelURL)
	return &client{
		remoteBaseURL: panelURL + "/api/remote",
		apiBaseURL:    panelURL + "/api/v1",
		token:         token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func normalizePanelBaseURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	value = strings.TrimSuffix(value, "/api/remote")
	value = strings.TrimSuffix(value, "/api/v1")
	return strings.TrimRight(value, "/")
}

// GetServerConfiguration fetches server config.
func (c *client) GetServerConfiguration(ctx context.Context, uuid string) (ServerConfigurationResponse, error) {
	var cfg ServerConfigurationResponse
	resp, err := c.get(ctx, "/servers/"+url.PathEscape(uuid))
	if err != nil {
		return cfg, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("decode server configuration: %w", err)
	}
	return cfg, nil
}

// GetServers fetches all servers.
func (c *client) GetServers(ctx context.Context, perPage int) ([]RawServerData, error) {
	var result struct {
		Data []RawServerData `json:"data"`
	}
	resp, err := c.get(ctx, fmt.Sprintf("/servers?per_page=%d", perPage))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode servers response: %w", err)
	}
	return result.Data, nil
}

// ResetServersState resets installing/restoring states.
func (c *client) ResetServersState(ctx context.Context) error {
	return c.postAndClose(ctx, c.remoteBaseURL, "/servers/reset", nil)
}

// SendActivityLogs sends activity to panel.
func (c *client) SendActivityLogs(ctx context.Context, activity []Activity) error {
	return c.postAndClose(ctx, c.remoteBaseURL, "/activity", map[string]interface{}{"data": activity})
}

func (c *client) get(ctx context.Context, path string) (*http.Response, error) {
	return c.request(ctx, http.MethodGet, c.remoteBaseURL, path, nil)
}

func (c *client) post(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	return c.request(ctx, http.MethodPost, c.remoteBaseURL, path, body)
}

func (c *client) postAPI(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	return c.request(ctx, http.MethodPost, c.apiBaseURL, path, body)
}

func (c *client) postAndClose(ctx context.Context, baseURL, path string, body interface{}) error {
	resp, err := c.request(ctx, http.MethodPost, baseURL, path, body)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func (c *client) request(ctx context.Context, method, baseURL, path string, body interface{}) (*http.Response, error) {
	var payload io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, fmt.Errorf("encode remote API request: %w", err)
		}
		payload = &buf
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return nil, fmt.Errorf("create remote API request: %w", err)
	}
	c.setHeaders(req)
	return c.do(req)
}

func (c *client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote API %s %s: %w", req.Method, req.URL.Path, err)
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return resp, nil
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	if readErr != nil {
		return nil, fmt.Errorf("remote API %s %s returned %s (read error body: %v)", req.Method, req.URL.Path, resp.Status, readErr)
	}
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		return nil, fmt.Errorf("remote API %s %s returned %s", req.Method, req.URL.Path, resp.Status)
	}
	return nil, fmt.Errorf("remote API %s %s returned %s: %s", req.Method, req.URL.Path, resp.Status, detail)
}

func (c *client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.forge.v1+json")
	req.Header.Set("Content-Type", "application/json")
}

// SendServerStats reports server resource usage.
func (c *client) SendServerStats(ctx context.Context, serverID string, stats ServerStats) error {
	return c.postAndClose(ctx, c.remoteBaseURL, "/servers/"+url.PathEscape(serverID)+"/stats", stats)
}

// SendNodeHeartbeat sends node heartbeat through Forge's /api/v1 route.
func (c *client) SendNodeHeartbeat(ctx context.Context, nodeID string, heartbeat NodeHeartbeat) error {
	resp, err := c.postAPI(ctx, "/nodes/"+url.PathEscape(nodeID)+"/heartbeat", heartbeat)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// CreatePlacementReservation creates a resource reservation.
func (c *client) CreatePlacementReservation(ctx context.Context, req PlacementReservationRequest) (PlacementReservation, error) {
	var reservation PlacementReservation
	resp, err := c.post(ctx, "/reservations", req)
	if err != nil {
		return reservation, err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&reservation); err != nil {
		return reservation, fmt.Errorf("decode placement reservation: %w", err)
	}
	return reservation, nil
}

// ConfirmPlacementReservation confirms a reservation.
func (c *client) ConfirmPlacementReservation(ctx context.Context, reservationID string) error {
	return c.postAndClose(ctx, c.remoteBaseURL, "/reservations/"+url.PathEscape(reservationID)+"/confirm", nil)
}

// CancelPlacementReservation cancels a reservation.
func (c *client) CancelPlacementReservation(ctx context.Context, reservationID string) error {
	return c.postAndClose(ctx, c.remoteBaseURL, "/reservations/"+url.PathEscape(reservationID)+"/cancel", nil)
}

// TriggerServerBackup initiates a backup.
func (c *client) TriggerServerBackup(ctx context.Context, serverID string) error {
	return c.postAndClose(ctx, c.remoteBaseURL, "/servers/"+url.PathEscape(serverID)+"/backups", nil)
}

// ReportEvacuationProgress reports evacuation status.
func (c *client) ReportEvacuationProgress(ctx context.Context, evacuationID string, progress EvacuationProgress) error {
	return c.postAndClose(ctx, c.remoteBaseURL, "/evacuations/"+url.PathEscape(evacuationID)+"/progress", progress)
}

// SetInstallationStatus notifies the panel that an installation completed.
func (c *client) SetInstallationStatus(ctx context.Context, serverID string, successful bool) error {
	body := map[string]any{
		"successful": successful,
		"reinstall":  false,
	}
	return c.postAndClose(ctx, c.remoteBaseURL, "/servers/"+url.PathEscape(serverID)+"/install", body)
}

// SendCrashEvent reports a server crash to the panel.
func (c *client) SendCrashEvent(ctx context.Context, serverID string, exitCode int, oomKilled bool, autoRestart bool) error {
	body := map[string]any{
		"exit_code":    exitCode,
		"oom_killed":   oomKilled,
		"auto_restart": autoRestart,
	}
	return c.postAndClose(ctx, c.remoteBaseURL, "/servers/"+url.PathEscape(serverID)+"/crash", body)
}

// SendBackupStatus notifies the panel that a backup completed.
func (c *client) SendBackupStatus(ctx context.Context, serverID string, req BackupStatusRequest) error {
	return c.postAndClose(ctx, c.remoteBaseURL, "/servers/"+url.PathEscape(serverID)+"/backups/status", req)
}

// SendRestoreStatus notifies the panel that a restore completed.
func (c *client) SendRestoreStatus(ctx context.Context, serverID string, req RestoreStatusRequest) error {
	return c.postAndClose(ctx, c.remoteBaseURL, "/servers/"+url.PathEscape(serverID)+"/backups/restore-status", req)
}
