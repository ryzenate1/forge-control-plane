package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type EdgeState string

const (
	EdgeStateConnected    EdgeState = "connected"
	EdgeStateDisconnected EdgeState = "disconnected"
	EdgeStateReconnecting EdgeState = "reconnecting"
	EdgeStateOffline      EdgeState = "offline"
)

type BackoffConfig struct {
	Initial time.Duration
	Max     time.Duration
	Factor  float64
	Jitter  bool
}

var DefaultBackoffConfig = BackoffConfig{
	Initial: 1 * time.Second,
	Max:     60 * time.Second,
	Factor:  2.0,
	Jitter:  true,
}

type EdgeAgent struct {
	panelURL      string
	nodeToken     string
	nodeID        string
	beaconVersion string

	state          EdgeState
	stateChangedAt time.Time
	mu             sync.RWMutex

	stopCh   chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once

	backoffCfg     BackoffConfig
	offlineTimeout time.Duration
	hbInterval     time.Duration

	lastHeartbeat   time.Time
	lastConnectTime time.Time
	connectAttempts int64
	reconnectCount  int64
	offlineDetected bool
	reconnecting    bool

	onConnect    func()
	onDisconnect func()

	httpClient *http.Client
}

func NewEdgeAgent(panelURL, nodeToken, nodeID, beaconVersion string) *EdgeAgent {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &EdgeAgent{
		panelURL:       panelURL,
		nodeToken:      nodeToken,
		nodeID:         nodeID,
		beaconVersion:  beaconVersion,
		state:          EdgeStateDisconnected,
		stateChangedAt: time.Now(),
		stopCh:         make(chan struct{}),
		stopped:        make(chan struct{}),
		backoffCfg:     DefaultBackoffConfig,
		offlineTimeout: 30 * time.Second,
		hbInterval:     15 * time.Second,
		httpClient:     &http.Client{Transport: transport, Timeout: 10 * time.Second},
	}
}

func (a *EdgeAgent) Start(ctx context.Context) {
	defer close(a.stopped)

	// The edge channel is only considered connected after a real round-trip to
	// the panel edge endpoint. Most panels do not implement /api/edge/connect,
	// so the agent stays honestly disconnected instead of reporting a fake
	// "connected" state.
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	connected := a.tryReconnect(probeCtx)
	cancel()
	if !connected {
		a.setState(EdgeStateDisconnected)
		log.Printf("[edge] edge channel unavailable on panel (no reachable /api/edge/connect endpoint); edge disabled")
		select {
		case <-ctx.Done():
		case <-a.stopCh:
		}
		return
	}

	a.mu.Lock()
	a.setStateLocked(EdgeStateConnected)
	a.lastConnectTime = time.Now()
	onConnect := a.onConnect
	a.mu.Unlock()
	if onConnect != nil {
		onConnect()
	}
	heartbeat := time.NewTicker(a.hbInterval)
	defer heartbeat.Stop()
	offlineCheck := time.NewTicker(a.offlineTimeout / 2)
	defer offlineCheck.Stop()
	for {
		select {
		case <-ctx.Done():
			a.setState(EdgeStateDisconnected)
			if a.onDisconnect != nil {
				a.onDisconnect()
			}
			return
		case <-a.stopCh:
			a.setState(EdgeStateDisconnected)
			if a.onDisconnect != nil {
				a.onDisconnect()
			}
			return
		case <-heartbeat.C:
			// Real heartbeat: re-verify the edge connection is still alive.
			if !a.tryReconnect(ctx) {
				a.mu.Lock()
				if !a.offlineDetected {
					a.offlineDetected = true
					a.setStateLocked(EdgeStateOffline)
				}
				a.mu.Unlock()
				log.Printf("[edge] heartbeat lost; entering reconnect loop")
				go a.reconnectLoop(ctx)
				continue
			}
			a.mu.Lock()
			a.lastHeartbeat = time.Now()
			a.mu.Unlock()
		case <-offlineCheck.C:
			a.mu.Lock()
			lastHB := a.lastHeartbeat
			offline := !lastHB.IsZero() && time.Since(lastHB) > a.offlineTimeout && !a.offlineDetected
			if offline {
				a.offlineDetected = true
				a.setStateLocked(EdgeStateOffline)
			}
			a.mu.Unlock()
			if offline {
				log.Printf("[edge] offline detected: no heartbeat for %v", time.Since(lastHB))
				go a.reconnectLoop(ctx)
			}
		}
	}
}

func (a *EdgeAgent) Stop() {
	a.stopOnce.Do(func() { close(a.stopCh) })
	<-a.stopped
}

func (a *EdgeAgent) reconnectLoop(ctx context.Context) {
	a.mu.Lock()
	if a.reconnecting {
		a.mu.Unlock()
		return
	}
	a.reconnecting = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.reconnecting = false
		a.mu.Unlock()
	}()
	backoff := a.backoffCfg.Initial
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		default:
		}
		a.mu.Lock()
		a.setStateLocked(EdgeStateReconnecting)
		a.reconnectCount++
		a.connectAttempts++
		attempt := a.connectAttempts
		a.mu.Unlock()
		log.Printf("[edge] reconnecting (attempt %d, backoff %v)...", attempt, backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		}
		if a.tryReconnect(ctx) {
			a.mu.Lock()
			a.setStateLocked(EdgeStateConnected)
			a.offlineDetected = false
			a.connectAttempts = 0
			a.lastConnectTime = time.Now()
			onConnect := a.onConnect
			a.mu.Unlock()
			if onConnect != nil {
				onConnect()
			}
			return
		}
		backoff = time.Duration(float64(backoff) * a.backoffCfg.Factor)
		if backoff > a.backoffCfg.Max {
			backoff = a.backoffCfg.Max
		}
		if a.backoffCfg.Jitter {
			jitter := secureJitter(backoff / 4)
			backoff = backoff - backoff/8 + jitter
		}
	}
}

type connectRequest struct {
	NodeID       string   `json:"nodeId"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type connectResponse struct {
	Connected bool   `json:"connected"`
	NodeID    string `json:"nodeId,omitempty"`
	Message   string `json:"message,omitempty"`
}

func (a *EdgeAgent) tryReconnect(ctx context.Context) bool {
	req := connectRequest{
		NodeID:  a.nodeID,
		Version: a.beaconVersion,
	}
	body, err := json.Marshal(req)
	if err != nil {
		log.Printf("[edge] reconnect marshal error: %v", err)
		return false
	}
	baseURL := normalizePanelBaseURL(a.panelURL)
	endpoint, err := url.Parse(baseURL + "/api/edge/connect")
	if err != nil || !secureEdgeURL(endpoint) {
		log.Printf("[edge] reconnect URL is invalid or insecure")
		return false
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		log.Printf("[edge] reconnect request error: %v", err)
		return false
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.nodeToken)
	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("[edge] reconnect http error: %v", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[edge] reconnect rejected: status %d", resp.StatusCode)
		return false
	}
	var cr connectResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		log.Printf("[edge] reconnect decode error: %v", err)
		return false
	}
	if !cr.Connected {
		log.Printf("[edge] reconnect denied: %s", cr.Message)
		return false
	}
	log.Printf("[edge] reconnected to panel as node %s", cr.NodeID)
	return true
}

func secureJitter(limit time.Duration) time.Duration {
	if limit <= 0 {
		return 0
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0
	}
	return time.Duration(value.Int64())
}

func normalizePanelBaseURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	value = strings.TrimSuffix(value, "/api/remote")
	value = strings.TrimSuffix(value, "/api/v1")
	return strings.TrimRight(value, "/")
}

func secureEdgeURL(endpoint *url.URL) bool {
	if endpoint == nil || endpoint.Hostname() == "" || endpoint.User != nil {
		return false
	}
	if endpoint.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(endpoint.Hostname())
	return endpoint.Scheme == "http" && (strings.EqualFold(endpoint.Hostname(), "localhost") || ip != nil && ip.IsLoopback())
}

// SetHTTPClient replaces the default HTTP client (for testing).
func (a *EdgeAgent) SetHTTPClient(client *http.Client) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.httpClient = client
}

// SetBackoffConfig overrides the default backoff configuration.
func (a *EdgeAgent) SetBackoffConfig(cfg BackoffConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.backoffCfg = cfg
}

// SetOfflineTimeout overrides the default offline detection timeout.
func (a *EdgeAgent) SetOfflineTimeout(timeout time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.offlineTimeout = timeout
}

// SetHBInterval overrides the default heartbeat interval.
func (a *EdgeAgent) SetHBInterval(interval time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.hbInterval = interval
}

// ReconnectCount returns the total reconnection attempts.
func (a *EdgeAgent) ReconnectCount() int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.reconnectCount
}

func (a *EdgeAgent) ConnectNow() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.offlineDetected = false
	a.setStateLocked(EdgeStateConnected)
	a.lastConnectTime = time.Now()
}

func (a *EdgeAgent) setState(s EdgeState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setStateLocked(s)
}

func (a *EdgeAgent) setStateLocked(s EdgeState) {
	if a.state != s {
		a.state = s
		a.stateChangedAt = time.Now()
	}
}

func (a *EdgeAgent) State() EdgeState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

func (a *EdgeAgent) StateDuration() time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return time.Since(a.stateChangedAt)
}

func (a *EdgeAgent) Stats() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return map[string]any{
		"state":            string(a.state),
		"stateDurationMs":  time.Since(a.stateChangedAt).Milliseconds(),
		"lastHeartbeatMs":  time.Since(a.lastHeartbeat).Milliseconds(),
		"lastConnectMs":    time.Since(a.lastConnectTime).Milliseconds(),
		"connectAttempts":  a.connectAttempts,
		"reconnectCount":   a.reconnectCount,
		"offlineDetected":  a.offlineDetected,
		"offlineTimeoutMs": a.offlineTimeout.Milliseconds(),
		"hbIntervalMs":     a.hbInterval.Milliseconds(),
	}
}
