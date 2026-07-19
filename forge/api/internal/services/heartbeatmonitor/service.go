package heartbeatmonitor

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type Config struct {
	WarningThreshold  time.Duration `json:"warningThreshold"`
	OfflineThreshold  time.Duration `json:"offlineThreshold"`
	RecoveryThreshold int           `json:"recoveryThreshold"`
	Interval          time.Duration `json:"interval"`
}

type Metrics struct {
	HeartbeatEvaluationsTotal uint64 `json:"heartbeat_evaluations_total"`
	NodesSuspectedTotal       uint64 `json:"nodes_suspected_total"`
	NodesOfflineTotal         uint64 `json:"nodes_offline_total"`
	NodesRecoveredTotal       uint64 `json:"nodes_recovered_total"`
}

type Evaluation struct {
	Node                    store.Node `json:"node"`
	PreviousState           string     `json:"previousState"`
	State                   string     `json:"state"`
	PreviousActualState     string     `json:"previousActualState"`
	ActualState             string     `json:"actualState"`
	Changed                 bool       `json:"changed"`
	LastSeenAt              *time.Time `json:"lastSeenAt,omitempty"`
	LastHeartbeatAgeSeconds int        `json:"lastHeartbeatAgeSeconds"`
	SuccessfulHeartbeats    int        `json:"successfulHeartbeats"`
	Reason                  string     `json:"reason"`
	Thresholds              Config     `json:"thresholds"`
}

type heartbeatStore interface {
	ListNodes(context.Context) ([]store.Node, error)
	GetNode(context.Context, string) (store.Node, error)
	ListNodeHeartbeatHistory(context.Context, string, int) ([]store.NodeHeartbeatHistory, error)
	SetNodeHeartbeatClassification(context.Context, string, store.NodeHeartbeatState, store.NodeActualState, int, string) (store.Node, store.Node, error)
}

type Service struct {
	store     heartbeatStore
	publisher events.Publisher
	config    Config
	mu        sync.Mutex
	metrics   Metrics
}

func DefaultConfig() Config {
	return Config{
		WarningThreshold:  30 * time.Second,
		OfflineThreshold:  90 * time.Second,
		RecoveryThreshold: 2,
		Interval:          30 * time.Second,
	}
}

func New(store *store.Store, publisher events.Publisher) *Service {
	return NewWithConfig(store, publisher, DefaultConfig())
}

func NewWithConfig(store heartbeatStore, publisher events.Publisher, config Config) *Service {
	return &Service{
		store:     store,
		publisher: publisher,
		config:    normalizeConfig(config),
	}
}

func (s *Service) Config() Config {
	if s == nil {
		return DefaultConfig()
	}
	return s.config
}

func (s *Service) Metrics() Metrics {
	if s == nil {
		return Metrics{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

func (s *Service) Start(ctx context.Context) {
	if s == nil || s.store == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(s.config.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.EvaluateAll(ctx)
			}
		}
	}()
}

func (s *Service) EvaluateAll(ctx context.Context) error {
	if s == nil || s.store == nil {
		return nil
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if _, err := s.evaluate(ctx, node, true); err != nil {
			continue
		}
	}
	return nil
}

func (s *Service) EvaluateNode(ctx context.Context, nodeID string) (Evaluation, error) {
	if s == nil || s.store == nil {
		return Evaluation{}, nil
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return Evaluation{}, err
	}
	return s.evaluate(ctx, node, true)
}

func (s *Service) InspectNode(ctx context.Context, nodeID string) (Evaluation, error) {
	if s == nil || s.store == nil {
		return Evaluation{}, nil
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return Evaluation{}, err
	}
	return s.evaluate(ctx, node, false)
}

func (s *Service) evaluate(ctx context.Context, node store.Node, persist bool) (Evaluation, error) {
	history, err := s.store.ListNodeHeartbeatHistory(ctx, node.ID, s.config.RecoveryThreshold+3)
	if err != nil {
		return Evaluation{}, err
	}
	// The store returns history ordered by observed_at DESC, id DESC.
	// Re-sort to guarantee most-recent-first order so classify() and
	// consecutiveSuccessfulHeartbeats() work correctly regardless of
	// store-level ordering changes.
	sort.SliceStable(history, func(i, j int) bool {
		return history[i].ObservedAt.After(history[j].ObservedAt)
	})
	state, actualState, recoveryCount, ageSeconds, reason := s.classify(node, history, time.Now().UTC())
	evaluation := Evaluation{
		Node:                    node,
		PreviousState:           node.HeartbeatState,
		State:                   string(state),
		PreviousActualState:     node.ActualState,
		ActualState:             string(actualState),
		Changed:                 node.HeartbeatState != string(state) || node.ActualState != string(actualState),
		LastSeenAt:              node.LastSeenAt,
		LastHeartbeatAgeSeconds: ageSeconds,
		SuccessfulHeartbeats:    recoveryCount,
		Reason:                  reason,
		Thresholds:              s.config,
	}
	if !persist {
		return evaluation, nil
	}

	s.increment(func(metrics *Metrics) {
		metrics.HeartbeatEvaluationsTotal++
	})
	previous, updated, err := s.store.SetNodeHeartbeatClassification(ctx, node.ID, state, actualState, recoveryCount, reason)
	if err != nil {
		return Evaluation{}, err
	}
	evaluation.Node = updated
	evaluation.PreviousState = previous.HeartbeatState
	evaluation.PreviousActualState = previous.ActualState
	evaluation.Changed = previous.HeartbeatState != string(state) || previous.ActualState != string(actualState)
	if evaluation.Changed {
		s.publishTransitions(ctx, previous, updated, evaluation)
	}
	return evaluation, nil
}

// classify evaluates the node's heartbeat state based on the last heartbeat age
// and the history of recent heartbeats.
// history must be ordered most-recent-first (index 0 = newest).
func (s *Service) classify(node store.Node, history []store.NodeHeartbeatHistory, now time.Time) (store.NodeHeartbeatState, store.NodeActualState, int, int, string) {
	successes := consecutiveSuccessfulHeartbeats(history)
	ageSeconds := 0
	if node.LastSeenAt == nil {
		return store.NodeHeartbeatStateOffline, store.NodeActualStateOffline, 0, ageSeconds, "node has never reported heartbeat"
	}
	age := now.Sub(*node.LastSeenAt)
	if age < 0 {
		age = 0
	}
	ageSeconds = int(age.Seconds())
	if len(history) > 0 && !history[0].Success && age < s.config.OfflineThreshold {
		return store.NodeHeartbeatStateSuspected, store.NodeActualStateDegraded, 0, ageSeconds, "latest heartbeat reported failure"
	}
	if age >= s.config.OfflineThreshold*2 {
		return store.NodeHeartbeatStateOffline, store.NodeActualStateOffline, 0, ageSeconds, "heartbeat expired beyond offline threshold"
	}
	if age >= s.config.OfflineThreshold {
		if node.HeartbeatState == string(store.NodeHeartbeatStateUnreachable) || node.HeartbeatState == string(store.NodeHeartbeatStateOffline) {
			return store.NodeHeartbeatStateOffline, store.NodeActualStateOffline, 0, ageSeconds, "heartbeat remained unreachable"
		}
		return store.NodeHeartbeatStateUnreachable, store.NodeActualStateDegraded, 0, ageSeconds, "heartbeat exceeded offline threshold"
	}
	if age >= s.config.WarningThreshold {
		return store.NodeHeartbeatStateSuspected, store.NodeActualStateDegraded, 0, ageSeconds, "heartbeat exceeded warning threshold"
	}
	if node.HeartbeatState == string(store.NodeHeartbeatStateOffline) ||
		node.HeartbeatState == string(store.NodeHeartbeatStateUnreachable) ||
		node.HeartbeatState == string(store.NodeHeartbeatStateSuspected) ||
		node.HeartbeatState == string(store.NodeHeartbeatStateRecovering) {
		if successes >= s.config.RecoveryThreshold {
			return store.NodeHeartbeatStateHealthy, store.NodeActualStateOnline, successes, ageSeconds, "recovery threshold satisfied"
		}
		return store.NodeHeartbeatStateRecovering, store.NodeActualStateDegraded, successes, ageSeconds, "successful heartbeat observed but recovery threshold not met"
	}
	return store.NodeHeartbeatStateHealthy, store.NodeActualStateOnline, successes, ageSeconds, "heartbeat healthy"
}

func (s *Service) publishTransitions(ctx context.Context, previous, updated store.Node, evaluation Evaluation) {
	if s.publisher == nil || previous.HeartbeatState == updated.HeartbeatState {
		if previous.ActualState != updated.ActualState {
			s.publishActualStateChanged(ctx, previous, updated, evaluation, nil)
		}
		return
	}
	var eventType events.EventType
	switch updated.HeartbeatState {
	case string(store.NodeHeartbeatStateSuspected):
		eventType = events.EventNodeSuspected
		s.increment(func(metrics *Metrics) {
			metrics.NodesSuspectedTotal++
		})
	case string(store.NodeHeartbeatStateUnreachable):
		eventType = events.EventNodeUnreachable
	case string(store.NodeHeartbeatStateOffline):
		eventType = events.EventNodeOffline
		s.increment(func(metrics *Metrics) {
			metrics.NodesOfflineTotal++
		})
	case string(store.NodeHeartbeatStateHealthy):
		eventType = events.EventNodeRecovered
		s.increment(func(metrics *Metrics) {
			metrics.NodesRecoveredTotal++
		})
	default:
		eventType = events.EventActualStateChanged
	}
	payload := s.transitionPayload(previous, updated, evaluation)
	_ = s.publisher.Publish(ctx, events.NewEnvelope(eventType, "heartbeat-monitor", "node", updated.ID, payload))
	s.publishActualStateChanged(ctx, previous, updated, evaluation, payload)
}

func (s *Service) publishActualStateChanged(ctx context.Context, previous, updated store.Node, evaluation Evaluation, payload map[string]any) {
	if s.publisher == nil || previous.ActualState == updated.ActualState {
		return
	}
	if payload == nil {
		payload = s.transitionPayload(previous, updated, evaluation)
	}
	_ = s.publisher.Publish(ctx, events.NewEnvelope(events.EventActualStateChanged, "heartbeat-monitor", "node", updated.ID, payload))
}

func (s *Service) transitionPayload(previous, updated store.Node, evaluation Evaluation) map[string]any {
	correlationID := uuid.NewString()
	return map[string]any{
		"correlationId":           correlationID,
		"previousHeartbeatState":  previous.HeartbeatState,
		"heartbeatState":          updated.HeartbeatState,
		"previousActualState":     previous.ActualState,
		"actualState":             updated.ActualState,
		"reason":                  evaluation.Reason,
		"lastSeenAt":              evaluation.LastSeenAt,
		"lastHeartbeatAgeSeconds": evaluation.LastHeartbeatAgeSeconds,
		"successfulHeartbeats":    evaluation.SuccessfulHeartbeats,
		"warningThresholdSeconds": int(s.config.WarningThreshold.Seconds()),
		"offlineThresholdSeconds": int(s.config.OfflineThreshold.Seconds()),
		"recoveryThreshold":       s.config.RecoveryThreshold,
	}
}

// consecutiveSuccessfulHeartbeats counts how many consecutive heartbeats
// from the most recent (history[0]) backwards are successful.
// history must be ordered most-recent-first (index 0 = newest).
func consecutiveSuccessfulHeartbeats(history []store.NodeHeartbeatHistory) int {
	count := 0
	for _, item := range history {
		if !item.Success {
			break
		}
		count++
	}
	return count
}

func normalizeConfig(config Config) Config {
	defaults := DefaultConfig()
	if config.WarningThreshold <= 0 {
		config.WarningThreshold = defaults.WarningThreshold
	}
	if config.OfflineThreshold <= 0 {
		config.OfflineThreshold = defaults.OfflineThreshold
	}
	if config.OfflineThreshold < config.WarningThreshold {
		config.OfflineThreshold = config.WarningThreshold
	}
	if config.RecoveryThreshold <= 0 {
		config.RecoveryThreshold = defaults.RecoveryThreshold
	}
	if config.Interval <= 0 {
		config.Interval = defaults.Interval
	}
	return config
}

func (s *Service) increment(update func(*Metrics)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.metrics)
}
