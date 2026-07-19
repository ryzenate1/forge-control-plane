package scheduler

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/store"
)

type PredictiveScorer struct {
	store             predictiveStore
	mu                sync.RWMutex
	metricsHistory    map[string][]ResourceMetric
	affinityRules     []AffinityRule
	antiAffinityRules []AntiAffinityRule
}

type ResourceMetric struct {
	Timestamp    time.Time `json:"timestamp"`
	CPUPercent   float64   `json:"cpuPercent"`
	MemoryUsedMB int       `json:"memoryUsedMb"`
	DiskUsedMB   int       `json:"diskUsedMb"`
	NetworkRx    int64     `json:"networkRx"`
	NetworkTx    int64     `json:"networkTx"`
	ServerCount  int       `json:"serverCount"`
}

type AffinityRule struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ServerID string  `json:"serverId,omitempty"`
	NodeID   string  `json:"nodeId,omitempty"`
	Label    string  `json:"label"`
	Weight   float64 `json:"weight"`
}

type AntiAffinityRule struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ServerID string  `json:"serverId,omitempty"`
	Label    string  `json:"label"`
	Scope    string  `json:"scope"`
	Weight   float64 `json:"weight"`
}

type PredictiveScore struct {
	NodeID            string  `json:"nodeId"`
	BaseScore         float64 `json:"baseScore"`
	TrendScore        float64 `json:"trendScore"`
	AffinityScore     float64 `json:"affinityScore"`
	AntiAffinityScore float64 `json:"antiAffinityScore"`
	TotalScore        float64 `json:"totalScore"`
	PredictedLoad     float64 `json:"predictedLoad"`
	Confidence        float64 `json:"confidence"`
}

type predictiveStore interface {
	NodeCapacitySnapshot(ctx context.Context, nodeID string) (store.NodeCapacitySnapshot, error)
	ListNodes(ctx context.Context) ([]store.Node, error)
	ListServersByNode(ctx context.Context, nodeID string) ([]store.Server, error)
}

func NewPredictiveScorer(store predictiveStore) *PredictiveScorer {
	return &PredictiveScorer{
		store:             store,
		metricsHistory:    make(map[string][]ResourceMetric),
		affinityRules:     make([]AffinityRule, 0),
		antiAffinityRules: make([]AntiAffinityRule, 0),
	}
}

func (s *PredictiveScorer) RecordMetric(ctx context.Context, nodeID string, metric ResourceMetric) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metricsHistory[nodeID] = append(s.metricsHistory[nodeID], metric)
	if len(s.metricsHistory[nodeID]) > 100 {
		s.metricsHistory[nodeID] = s.metricsHistory[nodeID][len(s.metricsHistory[nodeID])-100:]
	}
}

func (s *PredictiveScorer) PredictLoad(ctx context.Context, nodeID string) (float64, float64) {
	s.mu.RLock()
	metrics, ok := s.metricsHistory[nodeID]
	s.mu.RUnlock()

	if !ok || len(metrics) < 2 {
		return 0, 0
	}

	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].Timestamp.Before(metrics[j].Timestamp)
	})

	n := len(metrics)
	start := metrics[0].Timestamp

	var sumX, sumY, sumXY, sumX2 float64

	for _, m := range metrics {
		x := m.Timestamp.Sub(start).Seconds()
		y := m.CPUPercent
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	denom := float64(n)*sumX2 - sumX*sumX
	var slope, intercept float64
	if math.Abs(denom) > 1e-10 {
		slope = (float64(n)*sumXY - sumX*sumY) / denom
		intercept = (sumY - slope*sumX) / float64(n)
	} else {
		intercept = sumY / float64(n)
	}

	lastX := metrics[n-1].Timestamp.Sub(start).Seconds()
	futureX := lastX + 300
	predictedCPU := intercept + slope*futureX
	if predictedCPU < 0 {
		predictedCPU = 0
	}

	predictedLoad := predictedCPU / 100.0
	if predictedLoad > 1.0 {
		predictedLoad = 1.0
	}

	confidence := math.Min(1.0, float64(n)/10.0)

	return predictedLoad, confidence
}

func (s *PredictiveScorer) ScorePredictive(ctx context.Context, nodeID string, req domain.PlacementRequest) (*PredictiveScore, error) {
	snapshot, err := s.store.NodeCapacitySnapshot(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	baseScore := float64(snapshot.AvailableMemory)*1000000000 + float64(snapshot.AvailableCPU)*1000 + float64(snapshot.AvailableDisk)

	predictedLoad, confidence := s.PredictLoad(ctx, nodeID)
	var trendScore float64
	if confidence > 0 {
		trendScore = -predictedLoad * 0.5
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var affinityScore float64
	for _, rule := range s.affinityRules {
		if matchesAffinity(rule, req, nodeID) {
			affinityScore += rule.Weight
		}
	}

	var antiAffinityScore float64
	servers, _ := s.store.ListServersByNode(ctx, nodeID)
	for _, rule := range s.antiAffinityRules {
		if matchesAntiAffinity(ctx, rule, req, nodeID, servers) {
			antiAffinityScore += rule.Weight
		}
	}

	totalScore := baseScore*(1+trendScore) + affinityScore - antiAffinityScore

	return &PredictiveScore{
		NodeID:            nodeID,
		BaseScore:         baseScore,
		TrendScore:        trendScore,
		AffinityScore:     affinityScore,
		AntiAffinityScore: antiAffinityScore,
		TotalScore:        totalScore,
		PredictedLoad:     predictedLoad,
		Confidence:        confidence,
	}, nil
}

func matchesAffinity(rule AffinityRule, req domain.PlacementRequest, nodeID string) bool {
	if rule.ServerID != "" && rule.ServerID != req.ServerID {
		return false
	}
	if rule.NodeID != "" && rule.NodeID != nodeID {
		return false
	}
	return true
}

func matchesAntiAffinity(ctx context.Context, rule AntiAffinityRule, req domain.PlacementRequest, nodeID string, servers []store.Server) bool {
	if rule.ServerID != "" {
		if req.ServerID == rule.ServerID {
			for _, server := range servers {
				if server.ID == req.ServerID {
					return true
				}
			}
		}
		return false
	}
	if rule.Label != "" {
		for _, server := range servers {
			if server.ID != req.ServerID && rule.Scope == "node" {
				return true
			}
		}
	}
	return false
}

func (s *PredictiveScorer) AddAffinityRule(ctx context.Context, rule AffinityRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rule.ID == "" {
		rule.ID = generateID()
	}
	s.affinityRules = append(s.affinityRules, rule)
	return nil
}

func (s *PredictiveScorer) RemoveAffinityRule(ctx context.Context, ruleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, rule := range s.affinityRules {
		if rule.ID == ruleID {
			s.affinityRules = append(s.affinityRules[:i], s.affinityRules[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *PredictiveScorer) AddAntiAffinityRule(ctx context.Context, rule AntiAffinityRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rule.ID == "" {
		rule.ID = generateID()
	}
	s.antiAffinityRules = append(s.antiAffinityRules, rule)
	return nil
}

func (s *PredictiveScorer) RemoveAntiAffinityRule(ctx context.Context, ruleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, rule := range s.antiAffinityRules {
		if rule.ID == ruleID {
			s.antiAffinityRules = append(s.antiAffinityRules[:i], s.antiAffinityRules[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *PredictiveScorer) ListAffinityRules(ctx context.Context) ([]AffinityRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rules := make([]AffinityRule, len(s.affinityRules))
	copy(rules, s.affinityRules)
	return rules, nil
}

func (s *PredictiveScorer) ListAntiAffinityRules(ctx context.Context) ([]AntiAffinityRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rules := make([]AntiAffinityRule, len(s.antiAffinityRules))
	copy(rules, s.antiAffinityRules)
	return rules, nil
}

func generateID() string {
	return "rule-" + time.Now().Format("150405") + "-" + randomSuffix()
}

var idMu sync.Mutex
var idCounter int

func randomSuffix() string {
	idMu.Lock()
	defer idMu.Unlock()
	idCounter++
	return fmt.Sprintf("%06x", idCounter)
}
