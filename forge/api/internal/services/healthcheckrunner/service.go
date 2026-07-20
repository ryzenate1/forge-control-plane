package healthcheckrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type TargetStatus string

const (
	TargetStatusHealthy   TargetStatus = "healthy"
	TargetStatusSuspected TargetStatus = "suspected"
	TargetStatusUnhealthy TargetStatus = "unhealthy"
)

type CheckType string

const (
	CheckTypeTCP  CheckType = "tcp"
	CheckTypeHTTP CheckType = "http"
)

type HealthCheckConfig struct {
	Path               string `json:"path"`
	Port               int    `json:"port"`
	IntervalSeconds    int    `json:"intervalSeconds"`
	TimeoutSeconds     int    `json:"timeoutSeconds"`
	HealthyThreshold   int    `json:"healthyThreshold"`
	UnhealthyThreshold int    `json:"unhealthyThreshold"`
}

type Target struct {
	ID       string
	GroupID  string
	ServerID string
	NodeID   string
	IP       string
	Port     int
	Weight   int
}

type TargetHealthState struct {
	ID                     string
	GroupID                string
	ServerID               string
	Status                 TargetStatus
	ConsecutiveFailures    int
	ConsecutiveSuccesses   int
	SuspectedSince         *time.Time
	LastCheckAt            time.Time
	LastSuccessAt          *time.Time
	LastFailureAt          *time.Time
	HealthyThreshold       int
	UnhealthyThreshold     int
	mu                     sync.Mutex
}

type CheckResult struct {
	TargetID     string
	GroupID      string
	ServerID     string
	CheckType    CheckType
	Status       TargetStatus
	LatencyMs    int
	StatusCode   int
	ErrorMessage string
	CheckedAt    time.Time
}

type storeAdapter interface {
	ListTargetGroups(ctx context.Context) ([]store.TargetGroupRow, error)
	ListTargetsByGroup(ctx context.Context, groupID string) ([]store.TargetRow, error)
	UpdateTargetStatus(ctx context.Context, id string, status string) error
	InsertHealthCheckHistory(ctx context.Context, row store.HealthCheckHistoryRow) error
	PruneHealthCheckHistory(ctx context.Context, before time.Time) (int64, error)
	GetTargetHealthSummary(ctx context.Context, targetID string) (consecutiveFailures int, consecutiveSuccesses int, err error)
	ListServers(ctx context.Context) ([]store.Server, error)
	GetServer(ctx context.Context, serverID string) (store.Server, error)
}

type OnTargetUnhealthy func(ctx context.Context, serverID string, targetID string, consecutiveFailures int)

type Config struct {
	Interval        time.Duration
	HistoryRetention time.Duration
}

func DefaultConfig() Config {
	return Config{
		Interval:        15 * time.Second,
		HistoryRetention: 7 * 24 * time.Hour,
	}
}

type Service struct {
	store         storeAdapter
	config        Config
	states        map[string]*TargetHealthState
	mu            sync.RWMutex
	onUnhealthy   OnTargetUnhealthy
	lastGroupPoll time.Time
	lastGroups    []store.TargetGroupRow
	lastPrune     time.Time
}

func New(store storeAdapter, config Config) *Service {
	if config.Interval <= 0 {
		config.Interval = 15 * time.Second
	}
	if config.HistoryRetention <= 0 {
		config.HistoryRetention = 7 * 24 * time.Hour
	}
	return &Service{
		store:     store,
		config:    config,
		states:    make(map[string]*TargetHealthState),
		lastPrune: time.Now(),
	}
}

func (s *Service) OnUnhealthy(handler OnTargetUnhealthy) {
	s.onUnhealthy = handler
}

func (s *Service) Start(ctx context.Context) {
	if s == nil || s.store == nil {
		return
	}
	go func() {
		s.loadExistingStates(ctx)
		s.runOnce(ctx)
		ticker := time.NewTicker(s.config.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runOnce(ctx)
			}
		}
	}()
}

func (s *Service) loadExistingStates(ctx context.Context) {
	groups, err := s.store.ListTargetGroups(ctx)
	if err != nil {
		return
	}
	for _, g := range groups {
		targets, err := s.store.ListTargetsByGroup(ctx, g.ID)
		if err != nil {
			continue
		}
		for _, t := range targets {
			failures, successes, err := s.store.GetTargetHealthSummary(ctx, t.ID)
			if err != nil {
				continue
			}
			var status TargetStatus
			var suspectedSince *time.Time
			if t.Status == "unhealthy" {
				status = TargetStatusUnhealthy
			} else if t.Status == "draining" {
				status = TargetStatusUnhealthy
				now := time.Now()
				suspectedSince = &now
			} else if failures > 0 {
				status = TargetStatusSuspected
				now := time.Now().UTC()
				suspectedSince = &now
			} else {
				status = TargetStatusHealthy
			}
			var hc HealthCheckConfig
			if len(g.HealthCheck) > 0 {
				json.Unmarshal(g.HealthCheck, &hc)
			}
			healthyThreshold := hc.HealthyThreshold
			if healthyThreshold <= 0 {
				healthyThreshold = 3
			}
			unhealthyThreshold := hc.UnhealthyThreshold
			if unhealthyThreshold <= 0 {
				unhealthyThreshold = 3
			}
			s.mu.Lock()
			s.states[t.ID] = &TargetHealthState{
				ID:                   t.ID,
				GroupID:              g.ID,
				ServerID:             t.ServerID,
				Status:               status,
				ConsecutiveFailures:  failures,
				ConsecutiveSuccesses: successes,
				SuspectedSince:       suspectedSince,
				LastCheckAt:          time.Now(),
				HealthyThreshold:     healthyThreshold,
				UnhealthyThreshold:   unhealthyThreshold,
			}
			s.mu.Unlock()
		}
	}
}

func (s *Service) runOnce(ctx context.Context) {
	groups, err := s.store.ListTargetGroups(ctx)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.lastGroupPoll = time.Now()
	s.lastGroups = groups
	s.mu.Unlock()

	checkCtx, cancel := context.WithTimeout(ctx, s.config.Interval-time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for _, g := range groups {
		var cfg HealthCheckConfig
		if len(g.HealthCheck) == 0 {
			continue
		}
		if err := json.Unmarshal(g.HealthCheck, &cfg); err != nil {
			continue
		}
		if cfg.IntervalSeconds <= 0 {
			cfg.IntervalSeconds = 15
		}
		if cfg.TimeoutSeconds <= 0 {
			cfg.TimeoutSeconds = 5
		}
		if cfg.HealthyThreshold <= 0 {
			cfg.HealthyThreshold = 3
		}
		if cfg.UnhealthyThreshold <= 0 {
			cfg.UnhealthyThreshold = 3
		}
		targets, err := s.store.ListTargetsByGroup(checkCtx, g.ID)
		if err != nil {
			continue
		}
		for _, t := range targets {
			wg.Add(1)
			go func(target store.TargetRow, groupID string, hc HealthCheckConfig, protocol string) {
				defer wg.Done()
				s.checkTarget(checkCtx, target, groupID, hc, protocol)
			}(t, g.ID, cfg, g.Protocol)
		}
	}
	wg.Wait()

	if time.Since(s.lastPrune) > 1*time.Hour {
		_, _ = s.store.PruneHealthCheckHistory(ctx, time.Now().UTC().Add(-s.config.HistoryRetention))
		s.lastPrune = time.Now()
	}
}

func (s *Service) checkTarget(ctx context.Context, target store.TargetRow, groupID string, hc HealthCheckConfig, protocol string) {
	port := target.Port
	if hc.Port > 0 {
		port = hc.Port
	}

	stateKey := target.ID
	s.mu.RLock()
	state, exists := s.states[stateKey]
	s.mu.RUnlock()
	if !exists {
		state = &TargetHealthState{
			ID:                 target.ID,
			GroupID:            groupID,
			ServerID:           target.ServerID,
			Status:             TargetStatusHealthy,
			HealthyThreshold:   hc.HealthyThreshold,
			UnhealthyThreshold: hc.UnhealthyThreshold,
		}
	}

	state.mu.Lock()
	healthyThreshold := state.HealthyThreshold
	unhealthyThreshold := state.UnhealthyThreshold
	state.mu.Unlock()

	if healthyThreshold <= 0 {
		healthyThreshold = 3
	}
	if unhealthyThreshold <= 0 {
		unhealthyThreshold = 3
	}

	var result CheckResult
	checkType := CheckTypeTCP
	if protocol == "http" || protocol == "https" || hc.Path != "" {
		checkType = CheckTypeHTTP
	}

	switch checkType {
	case CheckTypeHTTP:
		result = s.runHTTPCheck(ctx, target, port, hc)
	default:
		result = s.runTCPCheck(ctx, target, port, hc)
	}

	result.CheckedAt = time.Now().UTC()

	if s.store != nil {
		_ = s.store.InsertHealthCheckHistory(ctx, store.HealthCheckHistoryRow{
			TargetID:     result.TargetID,
			GroupID:      result.GroupID,
			ServerID:     result.ServerID,
			CheckType:    string(result.CheckType),
			Status:       string(result.Status),
			LatencyMs:    result.LatencyMs,
			StatusCode:   result.StatusCode,
			ErrorMessage: result.ErrorMessage,
			CheckedAt:    result.CheckedAt,
		})
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	state.LastCheckAt = result.CheckedAt

	previousStatus := state.Status

	if result.Status == TargetStatusHealthy {
		state.ConsecutiveFailures = 0
		state.ConsecutiveSuccesses++
		state.LastSuccessAt = &result.CheckedAt

		if previousStatus == TargetStatusUnhealthy || previousStatus == TargetStatusSuspected {
			if state.ConsecutiveSuccesses >= healthyThreshold {
				state.Status = TargetStatusHealthy
				state.SuspectedSince = nil
				s.persistTargetStatus(ctx, target.ID, "healthy")
			}
		}
	} else {
		state.ConsecutiveSuccesses = 0
		state.ConsecutiveFailures++
		state.LastFailureAt = &result.CheckedAt

		if previousStatus == TargetStatusHealthy {
			if state.ConsecutiveFailures >= unhealthyThreshold/2 && state.ConsecutiveFailures < unhealthyThreshold {
				state.Status = TargetStatusSuspected
				now := time.Now().UTC()
				state.SuspectedSince = &now
				s.persistTargetStatus(ctx, target.ID, "draining")
			} else if state.ConsecutiveFailures >= unhealthyThreshold {
				state.Status = TargetStatusUnhealthy
				if state.SuspectedSince == nil {
					now := time.Now().UTC()
					state.SuspectedSince = &now
				}
				s.persistTargetStatus(ctx, target.ID, "unhealthy")
				if s.onUnhealthy != nil {
					go s.onUnhealthy(ctx, target.ServerID, target.ID, state.ConsecutiveFailures)
				}
			}
		} else if previousStatus == TargetStatusSuspected {
			if state.ConsecutiveFailures >= unhealthyThreshold {
				state.Status = TargetStatusUnhealthy
				s.persistTargetStatus(ctx, target.ID, "unhealthy")
				if s.onUnhealthy != nil {
					go s.onUnhealthy(ctx, target.ServerID, target.ID, state.ConsecutiveFailures)
				}
			}
		}
	}

	if !exists {
		s.mu.Lock()
		s.states[stateKey] = state
		s.mu.Unlock()
	}
}

func (s *Service) runHTTPCheck(ctx context.Context, target store.TargetRow, port int, hc HealthCheckConfig) CheckResult {
	result := CheckResult{
		TargetID:  target.ID,
		GroupID:   s.getGroupID(target.ID),
		ServerID:  target.ServerID,
		CheckType: CheckTypeHTTP,
	}

	timeout := time.Duration(hc.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	client := http.Client{Timeout: timeout}

	path := hc.Path
	if path == "" {
		path = "/"
	}

	url := fmt.Sprintf("http://%s:%d%s", target.IP, port, path)
	if target.IP == "" {
		url = fmt.Sprintf("http://%s:%d%s", "127.0.0.1", port, path)
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		result.Status = TargetStatusUnhealthy
		result.ErrorMessage = err.Error()
		return result
	}
	req.Header.Set("User-Agent", "GamePanel-HealthCheck/1.0")

	resp, err := client.Do(req)
	result.LatencyMs = int(time.Since(start).Milliseconds())

	if err != nil {
		result.Status = TargetStatusUnhealthy
		result.ErrorMessage = err.Error()
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		result.Status = TargetStatusHealthy
	} else {
		result.Status = TargetStatusUnhealthy
		result.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	return result
}

func (s *Service) runTCPCheck(ctx context.Context, target store.TargetRow, port int, hc HealthCheckConfig) CheckResult {
	result := CheckResult{
		TargetID:  target.ID,
		GroupID:   s.getGroupID(target.ID),
		ServerID:  target.ServerID,
		CheckType: CheckTypeTCP,
	}

	timeout := time.Duration(hc.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	address := net.JoinHostPort(target.IP, fmt.Sprintf("%d", port))
	if target.IP == "" {
		address = net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port))
	}

	start := time.Now()
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", address)
	if err != nil {
		result.Status = TargetStatusUnhealthy
		result.ErrorMessage = err.Error()
		result.LatencyMs = int(time.Since(start).Milliseconds())
		return result
	}
	_ = conn.Close()
	result.LatencyMs = int(time.Since(start).Milliseconds())
	result.Status = TargetStatusHealthy
	return result
}

func (s *Service) getGroupID(targetID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, state := range s.states {
		if state.ID == targetID {
			return state.GroupID
		}
	}
	return ""
}

func (s *Service) persistTargetStatus(ctx context.Context, targetID string, status string) {
	if s.store == nil {
		return
	}
	_ = s.store.UpdateTargetStatus(ctx, targetID, status)
}

func (s *Service) GetTargetState(targetID string) *TargetHealthState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.states[targetID]
}

func (s *Service) ListUnhealthyTargets(ctx context.Context) []*TargetHealthState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	results := make([]*TargetHealthState, 0)
	for _, state := range s.states {
		state.mu.Lock()
		if state.Status != TargetStatusHealthy {
			results = append(results, state)
		}
		state.mu.Unlock()
	}
	return results
}

func (s *Service) CorrelationID() string {
	return uuid.NewString()
}

func (s *Service) Metrics() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := len(s.states)
	healthy := 0
	suspected := 0
	unhealthy := 0
	for _, state := range s.states {
		state.mu.Lock()
		switch state.Status {
		case TargetStatusHealthy:
			healthy++
		case TargetStatusSuspected:
			suspected++
		case TargetStatusUnhealthy:
			unhealthy++
		}
		state.mu.Unlock()
	}
	return map[string]any{
		"total":     total,
		"healthy":   healthy,
		"suspected": suspected,
		"unhealthy": unhealthy,
	}
}
