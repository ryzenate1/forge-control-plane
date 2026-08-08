package remote

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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
	SendCapabilityReport(ctx context.Context, report interface{}) error
}

type client struct {
	remoteBaseURL string
	apiBaseURL    string
	token         string
	httpClient    *http.Client
	initErr       error
}

// NewClient accepts the panel root URL (or a URL ending in /api/v1 or
// /api/remote) and derives the two API roots deliberately. Remote daemon calls
// use /api/remote, while node heartbeat uses the Forge /api/v1 route.
func NewClient(panelURL, token string) Client {
	panelURL = normalizePanelBaseURL(panelURL)
	parsed, err := url.Parse(panelURL)
	if err == nil {
		err = validatePanelEndpoint(parsed)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.MaxIdleConnsPerHost = 10
	transport.MaxIdleConns = 50
	result := &client{
		remoteBaseURL: panelURL + "/api/remote",
		apiBaseURL:    panelURL + "/api/v1",
		token:         token,
		initErr:       err,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
		},
	}
	result.httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many panel redirects")
		}
		if len(via) > 0 && !sameEndpointOrigin(via[0].URL, req.URL) {
			return errors.New("cross-origin panel redirect refused")
		}
		return validatePanelEndpoint(req.URL)
	}
	return result
}

func validatePanelEndpoint(endpoint *url.URL) error {
	if endpoint == nil || endpoint.Hostname() == "" || endpoint.User != nil {
		return errors.New("panel URL must include a host and no credentials")
	}
	if endpoint.Scheme == "https" {
		return nil
	}
	ip := net.ParseIP(endpoint.Hostname())
	if endpoint.Scheme == "http" && (strings.EqualFold(endpoint.Hostname(), "localhost") || ip != nil && ip.IsLoopback()) {
		return nil
	}
	return errors.New("panel URL must use HTTPS (HTTP is allowed only for loopback)")
}

func sameEndpointOrigin(left, right *url.URL) bool {
	return left != nil && right != nil &&
		strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Host, right.Host)
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
	if c.initErr != nil {
		return nil, c.initErr
	}
	var payload []byte
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, fmt.Errorf("encode remote API request: %w", err)
		}
		payload = buf.Bytes()
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("create remote API request: %w", err)
		}
		if err := c.setHeaders(req, payload); err != nil {
			return nil, err
		}
		resp, err := c.httpClient.Do(req)
		if err == nil && resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return c.handleResponse(req, resp)
		}
		if err == nil && attempt == 2 {
			return c.handleResponse(req, resp)
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("remote API %s %s returned %s", method, req.URL.Path, resp.Status)
		} else {
			lastErr = err
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(1<<attempt) * 250 * time.Millisecond)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}
	}
	return nil, fmt.Errorf("remote API request failed after retries: %w", lastErr)
}

func (c *client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote API %s %s: %w", req.Method, req.URL.Path, err)
	}
	return c.handleResponse(req, resp)
}

func (c *client) handleResponse(req *http.Request, resp *http.Response) (*http.Response, error) {
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

func (c *client) setHeaders(req *http.Request, body []byte) error {
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return fmt.Errorf("generate panel request nonce: %w", err)
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	nonce := hex.EncodeToString(nonceBytes)
	mac := hmac.New(sha256.New, []byte(c.token))
	_, _ = io.WriteString(mac, req.Method+"\n"+req.URL.RequestURI()+"\n"+timestamp+"\n"+nonce+"\n")
	_, _ = mac.Write(body)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.forge.v1+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Panel-Timestamp", timestamp)
	req.Header.Set("X-Panel-Nonce", nonce)
	req.Header.Set("X-Panel-Signature", hex.EncodeToString(mac.Sum(nil)))
	return nil
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

// SendCapabilityReport sends a capability report to the panel.
func (c *client) SendCapabilityReport(ctx context.Context, report interface{}) error {
	return c.postAndClose(ctx, c.apiBaseURL, "/nodes/capabilities", report)
}
