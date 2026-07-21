package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
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
	panelURL       string
	nodeToken      string
	nodeID         string
	beaconVersion  string

	state          EdgeState
	stateChangedAt time.Time
	mu             sync.RWMutex

	stopCh         chan struct{}
	stopped        chan struct{}

	backoffCfg     BackoffConfig
	offlineTimeout time.Duration
	hbInterval     time.Duration

	lastHeartbeat    time.Time
	lastConnectTime  time.Time
	connectAttempts  int64
	reconnectCount   int64
	offlineDetected  bool

	onConnect    func()
	onDisconnect func()

	httpClient *http.Client
}

// SetEdgeAgent sets the package-level edge agent instance used by HTTP
// handlers. Called from main.go when panel onboarding is enabled.
func SetEdgeAgent(agent *EdgeAgent) {
	edgeAgentMu.Lock()
	defer edgeAgentMu.Unlock()
	edgeAgent = agent
}

func NewEdgeAgent(panelURL, nodeToken, nodeID, beaconVersion string) *EdgeAgent {
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
		httpClient:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *EdgeAgent) Start(ctx context.Context) {
	defer close(a.stopped)
	a.setState(EdgeStateConnected)
	a.lastConnectTime = time.Now()
	if a.onConnect != nil {
		a.onConnect()
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
			a.mu.Lock()
			a.lastHeartbeat = time.Now()
			a.mu.Unlock()
		case <-offlineCheck.C:
			a.mu.RLock()
			lastHB := a.lastHeartbeat
			a.mu.RUnlock()
			if !lastHB.IsZero() && time.Since(lastHB) > a.offlineTimeout && !a.offlineDetected {
				a.mu.Lock()
				a.offlineDetected = true
				a.setStateLocked(EdgeStateOffline)
				a.mu.Unlock()
				log.Printf("[edge] offline detected: no heartbeat for %v", time.Since(lastHB))
				go a.reconnectLoop(ctx)
			}
		}
	}
}

func (a *EdgeAgent) Stop() {
	close(a.stopCh)
	<-a.stopped
}

func (a *EdgeAgent) reconnectLoop(ctx context.Context) {
	backoff := a.backoffCfg.Initial
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		default:
		}
		a.setState(EdgeStateReconnecting)
		a.reconnectCount++
		a.connectAttempts++
		log.Printf("[edge] reconnecting (attempt %d, backoff %v)...", a.connectAttempts, backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		}
		if a.tryReconnect() {
			a.setState(EdgeStateConnected)
			a.offlineDetected = false
			a.connectAttempts = 0
			a.lastConnectTime = time.Now()
			if a.onConnect != nil {
				a.onConnect()
			}
			return
		}
		backoff = time.Duration(float64(backoff) * a.backoffCfg.Factor)
		if backoff > a.backoffCfg.Max {
			backoff = a.backoffCfg.Max
		}
		if a.backoffCfg.Jitter {
			jitter := time.Duration(rand.Int63n(int64(backoff) / 4))
			backoff = backoff - backoff/8 + jitter
		}
	}
}

type connectRequest struct {
	NodeID     string `json:"nodeId"`
	Token      string `json:"token"`
	Version    string `json:"version"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type connectResponse struct {
	Connected bool   `json:"connected"`
	NodeID    string `json:"nodeId,omitempty"`
	Message   string `json:"message,omitempty"`
}

func (a *EdgeAgent) tryReconnect() bool {
	req := connectRequest{
		NodeID:    a.nodeID,
		Token:     a.nodeToken,
		Version:   a.beaconVersion,
	}
	body, err := json.Marshal(req)
	if err != nil {
		log.Printf("[edge] reconnect marshal error: %v", err)
		return false
	}
	baseURL := strings.TrimRight(a.panelURL, "/")
	httpReq, err := http.NewRequest(http.MethodPost, baseURL+"/api/edge/connect", bytes.NewReader(body))
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

// SetHTTPClient replaces the default HTTP client (for testing).
func (a *EdgeAgent) SetHTTPClient(client *http.Client) {
	a.httpClient = client
}

// SetBackoffConfig overrides the default backoff configuration.
func (a *EdgeAgent) SetBackoffConfig(cfg BackoffConfig) {
	a.backoffCfg = cfg
}

// SetOfflineTimeout overrides the default offline detection timeout.
func (a *EdgeAgent) SetOfflineTimeout(timeout time.Duration) {
	a.offlineTimeout = timeout
}

// SetHBInterval overrides the default heartbeat interval.
func (a *EdgeAgent) SetHBInterval(interval time.Duration) {
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
		"state":           string(a.state),
		"stateDurationMs": time.Since(a.stateChangedAt).Milliseconds(),
		"lastHeartbeatMs": time.Since(a.lastHeartbeat).Milliseconds(),
		"lastConnectMs":   time.Since(a.lastConnectTime).Milliseconds(),
		"connectAttempts": a.connectAttempts,
		"reconnectCount":  a.reconnectCount,
		"offlineDetected": a.offlineDetected,
		"offlineTimeoutMs": a.offlineTimeout.Milliseconds(),
		"hbIntervalMs":    a.hbInterval.Milliseconds(),
	}
}
