package loadbalancer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type Algorithm string

const (
	AlgorithmRoundRobin         Algorithm = "round_robin"
	AlgorithmLeastConnections   Algorithm = "least_connections"
	AlgorithmIPHash             Algorithm = "ip_hash"
	AlgorithmWeightedRoundRobin Algorithm = "weighted_round_robin"
)

type TargetStatus string

const (
	TargetStatusHealthy   TargetStatus = "healthy"
	TargetStatusUnhealthy TargetStatus = "unhealthy"
	TargetStatusDraining  TargetStatus = "draining"
)

type TargetGroup struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Algorithm   Algorithm          `json:"algorithm"`
	Port        int                `json:"port"`
	Protocol    string             `json:"protocol"`
	HealthCheck *HealthCheckConfig `json:"healthCheck,omitempty"`
	Targets     []Target           `json:"targets"`
	CreatedAt   time.Time          `json:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt"`
}

type Target struct {
	ID          string       `json:"id"`
	ServerID    string       `json:"serverId"`
	NodeID      string       `json:"nodeId"`
	IP          string       `json:"ip"`
	Port        int          `json:"port"`
	Weight      int          `json:"weight"`
	Status      TargetStatus `json:"status"`
	Connections int          `json:"connections"`
}

type HealthCheckConfig struct {
	Path               string `json:"path"`
	Port               int    `json:"port"`
	IntervalSeconds    int    `json:"intervalSeconds"`
	TimeoutSeconds     int    `json:"timeoutSeconds"`
	HealthyThreshold   int    `json:"healthyThreshold"`
	UnhealthyThreshold int    `json:"unhealthyThreshold"`
}

type Service struct {
	mu         sync.RWMutex
	groups     map[string]*TargetGroup
	roundRobin map[string]int
	connCount  map[string]int
	publisher  events.Publisher
	store      *store.Store
	bindHost   string
	portMin    int
	portMax    int
	enabled    bool
	listeners  map[string]net.Listener
	packets    map[string]net.PacketConn
	cancel     context.CancelFunc
}

var (
	ErrGroupNotFound   = errors.New("target group not found")
	ErrTargetNotFound  = errors.New("target not found")
	ErrNoHealthyTarget = errors.New("no healthy targets available")
)

func New(store *store.Store, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	svc := &Service{
		groups:     make(map[string]*TargetGroup),
		roundRobin: make(map[string]int),
		connCount:  make(map[string]int),
		publisher:  publisher,
		store:      store,
		bindHost:   strings.TrimSpace(os.Getenv("LOAD_BALANCER_BIND_HOST")),
		portMin:    loadBalancerEnvInt("LOAD_BALANCER_PORT_MIN", 30000),
		portMax:    loadBalancerEnvInt("LOAD_BALANCER_PORT_MAX", 30100),
		enabled:    strings.EqualFold(strings.TrimSpace(os.Getenv("LOAD_BALANCER_ENABLED")), "true"),
		listeners:  make(map[string]net.Listener),
		packets:    make(map[string]net.PacketConn),
	}
	if store != nil {
		svc.loadFromDB(context.Background())
	}
	return svc
}

func (s *Service) loadFromDB(ctx context.Context) {
	groupRows, err := s.store.ListTargetGroups(ctx)
	if err != nil {
		return
	}
	for _, row := range groupRows {
		group := &TargetGroup{
			ID:        row.ID,
			Name:      row.Name,
			Algorithm: Algorithm(row.Algorithm),
			Port:      row.Port,
			Protocol:  row.Protocol,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		}
		if len(row.HealthCheck) > 0 {
			var hc HealthCheckConfig
			if json.Unmarshal(row.HealthCheck, &hc) == nil {
				group.HealthCheck = &hc
			}
		}
		if group.Targets == nil {
			group.Targets = make([]Target, 0)
		}
		s.groups[row.ID] = group
		s.roundRobin[row.ID] = 0

		targetRows, err := s.store.ListTargetsByGroup(ctx, row.ID)
		if err != nil {
			continue
		}
		for _, tr := range targetRows {
			group.Targets = append(group.Targets, Target{
				ID:          tr.ID,
				ServerID:    tr.ServerID,
				NodeID:      tr.NodeID,
				IP:          tr.IP,
				Port:        tr.Port,
				Weight:      tr.Weight,
				Status:      TargetStatus(tr.Status),
				Connections: tr.Connections,
			})
		}
	}
}

func validateTargetGroup(group *TargetGroup) error {
	if group == nil {
		return errors.New("target group is required")
	}
	if group.ID == "" {
		return errors.New("target group id is required")
	}
	if group.Name == "" {
		return errors.New("target group name is required")
	}
	if group.Port < 1 || group.Port > 65535 {
		return errors.New("a valid port is required")
	}
	if group.Algorithm == "" {
		group.Algorithm = AlgorithmRoundRobin
	}
	switch group.Algorithm {
	case AlgorithmRoundRobin, AlgorithmLeastConnections, AlgorithmIPHash, AlgorithmWeightedRoundRobin:
	default:
		return errors.New("unsupported load-balancing algorithm")
	}
	if group.Protocol == "" {
		group.Protocol = "tcp"
	}
	group.Protocol = strings.ToLower(strings.TrimSpace(group.Protocol))
	if group.Protocol != "tcp" && group.Protocol != "udp" {
		return errors.New("protocol must be tcp or udp")
	}
	return nil
}

func (s *Service) CreateTargetGroup(ctx context.Context, group *TargetGroup) error {
	if err := validateTargetGroup(group); err != nil {
		return err
	}
	if s.enabled && (group.Port < s.portMin || group.Port > s.portMax) {
		return fmt.Errorf("listener port must be between %d and %d", s.portMin, s.portMax)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	group.CreatedAt = time.Now().UTC()
	group.UpdatedAt = time.Now().UTC()
	if group.Targets == nil {
		group.Targets = make([]Target, 0)
	}

	if s.store != nil {
		var hc []byte
		if group.HealthCheck != nil {
			hc, _ = json.Marshal(group.HealthCheck)
		}
		if err := s.store.CreateTargetGroup(ctx, store.TargetGroupRow{
			ID:          group.ID,
			Name:        group.Name,
			Algorithm:   string(group.Algorithm),
			Port:        group.Port,
			Protocol:    group.Protocol,
			HealthCheck: hc,
			CreatedAt:   group.CreatedAt,
			UpdatedAt:   group.UpdatedAt,
		}); err != nil {
			return err
		}
	}

	s.groups[group.ID] = group
	s.roundRobin[group.ID] = 0

	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope("target_group_created", "loadbalancer", "group", group.ID, map[string]any{
			"name": group.Name, "algorithm": group.Algorithm,
		}))
	}
	return nil
}

func (s *Service) GetTargetGroup(ctx context.Context, groupID string) (*TargetGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	group, exists := s.groups[groupID]
	if !exists {
		return nil, ErrGroupNotFound
	}
	return group, nil
}

func (s *Service) UpdateTargetGroup(ctx context.Context, group *TargetGroup) error {
	if err := validateTargetGroup(group); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.groups[group.ID]; !exists {
		return ErrGroupNotFound
	}
	group.UpdatedAt = time.Now().UTC()

	if s.store != nil {
		var hc []byte
		if group.HealthCheck != nil {
			hc, _ = json.Marshal(group.HealthCheck)
		}
		if err := s.store.UpdateTargetGroup(ctx, store.TargetGroupRow{
			ID:          group.ID,
			Name:        group.Name,
			Algorithm:   string(group.Algorithm),
			Port:        group.Port,
			Protocol:    group.Protocol,
			HealthCheck: hc,
			UpdatedAt:   group.UpdatedAt,
		}); err != nil {
			return err
		}
	}

	s.closeListenerLocked(group.ID)
	s.groups[group.ID] = group
	return nil
}

func (s *Service) DeleteTargetGroup(ctx context.Context, groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.groups[groupID]; !exists {
		return ErrGroupNotFound
	}

	if s.store != nil {
		if err := s.store.DeleteTargetGroup(ctx, groupID); err != nil {
			return err
		}
	}

	delete(s.groups, groupID)
	delete(s.roundRobin, groupID)
	s.closeListenerLocked(groupID)
	return nil
}

func loadBalancerEnvInt(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value < 1 || value > 65535 {
		return fallback
	}
	return value
}

func (s *Service) ListGroups(ctx context.Context) ([]*TargetGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*TargetGroup, 0, len(s.groups))
	for _, g := range s.groups {
		result = append(result, g)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s *Service) AddTarget(ctx context.Context, groupID, serverID, nodeID, ip string, port, weight int) (*Target, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, exists := s.groups[groupID]
	if !exists {
		return nil, ErrGroupNotFound
	}

	target := Target{
		ID:       fmt.Sprintf("t-%s-%s-%d", groupID, serverID, port),
		ServerID: serverID,
		NodeID:   nodeID,
		IP:       ip,
		Port:     port,
		Weight:   weight,
		Status:   TargetStatusHealthy,
	}
	if target.Weight <= 0 {
		target.Weight = 1
	}

	if s.store != nil {
		if err := s.store.CreateTarget(ctx, store.TargetRow{
			ID:       target.ID,
			GroupID:  groupID,
			ServerID: serverID,
			NodeID:   nodeID,
			IP:       ip,
			Port:     port,
			Weight:   target.Weight,
			Status:   string(target.Status),
		}); err != nil {
			return nil, err
		}
	}

	group.Targets = append(group.Targets, target)
	group.UpdatedAt = time.Now().UTC()

	return &target, nil
}

func (s *Service) RemoveTarget(ctx context.Context, groupID, targetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	group, exists := s.groups[groupID]
	if !exists {
		return ErrGroupNotFound
	}

	if s.store != nil {
		if err := s.store.DeleteTarget(ctx, targetID); err != nil {
			return err
		}
	}

	for i, t := range group.Targets {
		if t.ID == targetID {
			group.Targets = append(group.Targets[:i], group.Targets[i+1:]...)
			group.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return ErrTargetNotFound
}

func (s *Service) NextTarget(ctx context.Context, groupID, clientIP string) (*Target, error) {
	s.mu.RLock()
	group, exists := s.groups[groupID]
	var targets []Target
	var algorithm Algorithm
	if exists {
		targets = append([]Target(nil), group.Targets...)
		algorithm = group.Algorithm
	}
	s.mu.RUnlock()

	if !exists {
		return nil, ErrGroupNotFound
	}

	var healthyTargets []Target
	for _, t := range targets {
		if t.Status == TargetStatusHealthy {
			healthyTargets = append(healthyTargets, t)
		}
	}

	if len(healthyTargets) == 0 {
		return nil, ErrNoHealthyTarget
	}

	switch algorithm {
	case AlgorithmRoundRobin:
		return s.nextRoundRobin(groupID, healthyTargets), nil
	case AlgorithmLeastConnections:
		return s.nextLeastConnections(healthyTargets), nil
	case AlgorithmIPHash:
		return s.nextIPHash(healthyTargets, clientIP), nil
	case AlgorithmWeightedRoundRobin:
		return s.nextWeightedRoundRobin(healthyTargets), nil
	default:
		return s.nextRoundRobin(groupID, healthyTargets), nil
	}
}

func (s *Service) nextRoundRobin(groupID string, targets []Target) *Target {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.roundRobin[groupID] % len(targets)
	s.roundRobin[groupID] = (idx + 1) % len(targets)
	return &targets[idx]
}

func (s *Service) nextLeastConnections(targets []Target) *Target {
	s.mu.Lock()
	defer s.mu.Unlock()

	minConn := -1
	var selected *Target
	for i := range targets {
		conn := s.connCount[targets[i].ID]
		if minConn == -1 || conn < minConn {
			minConn = conn
			selected = &targets[i]
		}
	}
	if selected != nil {
		s.connCount[selected.ID]++
	}
	return selected
}

func (s *Service) nextIPHash(targets []Target, clientIP string) *Target {
	if clientIP == "" {
		return &targets[0]
	}
	h := fnv.New32a()
	h.Write([]byte(clientIP))
	idx := int(h.Sum32()) % len(targets)
	return &targets[idx]
}

func (s *Service) nextWeightedRoundRobin(targets []Target) *Target {
	totalWeight := 0
	for _, t := range targets {
		totalWeight += t.Weight
	}
	if totalWeight == 0 {
		return &targets[0]
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.roundRobin["__weighted"]
	idx = (idx + 1) % totalWeight
	s.roundRobin["__weighted"] = idx

	cumulative := 0
	for _, t := range targets {
		cumulative += t.Weight
		if idx < cumulative {
			return &t
		}
	}
	return &targets[0]
}

func (s *Service) ReleaseConnection(targetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connCount[targetID] > 0 {
		s.connCount[targetID]--
	}
}

func (s *Service) SetTargetStatus(ctx context.Context, groupID, targetID string, status TargetStatus) error {
	if status != TargetStatusHealthy && status != TargetStatusUnhealthy && status != TargetStatusDraining {
		return errors.New("invalid target status")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	group, exists := s.groups[groupID]
	if !exists {
		return ErrGroupNotFound
	}

	if s.store != nil {
		if err := s.store.UpdateTargetStatus(ctx, targetID, string(status)); err != nil {
			return err
		}
	}

	for i := range group.Targets {
		if group.Targets[i].ID == targetID {
			group.Targets[i].Status = status
			group.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return ErrTargetNotFound
}

func (s *Service) Metrics(ctx context.Context) map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalTargets := 0
	healthyTargets := 0
	for _, g := range s.groups {
		totalTargets += len(g.Targets)
		for _, t := range g.Targets {
			if t.Status == TargetStatusHealthy {
				healthyTargets++
			}
		}
	}

	return map[string]any{
		"groups":         len(s.groups),
		"totalTargets":   totalTargets,
		"healthyTargets": healthyTargets,
	}
}

func (s *Service) Handle(ctx context.Context, envelope events.Envelope) error {
	switch envelope.Type {
	case events.EventNodeOffline:
		return s.MarkNodeTargetsUnhealthy(ctx, envelope.ResourceID)
	case events.EventNodeRecovered:
		return s.MarkNodeTargetsHealthy(ctx, envelope.ResourceID)
	default:
		return nil
	}
}

func (s *Service) MarkNodeTargetsUnhealthy(ctx context.Context, nodeID string) error {
	if s.store == nil {
		return nil
	}
	if _, err := s.store.GetNode(ctx, nodeID); err != nil {
		return err
	}
	targets, err := s.store.ListTargetsByNode(ctx, nodeID)
	if err != nil {
		return err
	}
	count := 0
	for _, t := range targets {
		s.mu.Lock()
		group, exists := s.groups[t.GroupID]
		if exists {
			for i := range group.Targets {
				if group.Targets[i].ID == t.ID {
					group.Targets[i].Status = TargetStatusUnhealthy
					group.UpdatedAt = time.Now().UTC()
					if err := s.store.UpdateTargetStatus(ctx, t.ID, string(TargetStatusUnhealthy)); err != nil {
						slog.Error("loadbalancer: failed to persist target status", "targetId", t.ID, "error", err)
					} else {
						count++
					}
					break
				}
			}
		}
		s.mu.Unlock()
	}
	if count > 0 {
		slog.Info("marked targets unhealthy due to node going offline", "nodeId", nodeID, "count", count)
	}
	return nil
}

func (s *Service) MarkNodeTargetsHealthy(ctx context.Context, nodeID string) error {
	if s.store == nil {
		return nil
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		return err
	}
	if node.HeartbeatState != string(store.NodeHeartbeatStateHealthy) {
		return nil
	}
	targets, err := s.store.ListTargetsByNode(ctx, nodeID)
	if err != nil {
		return err
	}
	count := 0
	for _, t := range targets {
		s.mu.Lock()
		group, exists := s.groups[t.GroupID]
		if exists {
			for i := range group.Targets {
				if group.Targets[i].ID == t.ID {
					group.Targets[i].Status = TargetStatusHealthy
					group.UpdatedAt = time.Now().UTC()
					if err := s.store.UpdateTargetStatus(ctx, t.ID, string(TargetStatusHealthy)); err != nil {
						slog.Error("loadbalancer: failed to persist target status", "targetId", t.ID, "error", err)
					} else {
						count++
					}
					break
				}
			}
		}
		s.mu.Unlock()
	}
	if count > 0 {
		slog.Info("marked targets healthy due to node recovery", "nodeId", nodeID, "count", count)
	}
	return nil
}
