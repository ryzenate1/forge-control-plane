package scheduler

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/placement"
	"gamepanel/forge/internal/store"
)

type NodeScore struct {
	Node   store.Node `json:"node"`
	Score  float64    `json:"score"`
	Reason string     `json:"reason"`
}

type Service interface {
	PlaceServer(context.Context, domain.PlacementRequest) (domain.PlacementDecision, error)
	FilterNodes(context.Context, domain.PlacementRequest, []store.Node) ([]store.Node, error)
	ScoreNodes(context.Context, domain.PlacementRequest, []store.Node) ([]NodeScore, error)
}

type Scheduler struct {
	store     *store.Store
	engine    *placement.Engine
	publisher events.Publisher
	mu        sync.Mutex
	metrics   Metrics
}

type Metrics struct {
	PlacementRejectionsTotal uint64 `json:"placement_rejections_total"`
	CapacityExceededTotal    uint64 `json:"capacity_exceeded_total"`
}

func New(store *store.Store, engine *placement.Engine, publishers ...events.Publisher) *Scheduler {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Scheduler{store: store, engine: engine, publisher: publisher}
}

func (s *Scheduler) Metrics() Metrics {
	if s == nil {
		return Metrics{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

func (s *Scheduler) PlaceServer(ctx context.Context, req domain.PlacementRequest) (domain.PlacementDecision, error) {
	req = normalizeRequest(req)
	if req.RegionID != "" {
		resolved, err := s.resolveRegionID(ctx, req.RegionID)
		if err != nil {
			return domain.PlacementDecision{}, err
		}
		if resolved != "" {
			req.RegionID = resolved
		}
	}
	if req.RegionID == "" && req.RequiredNode == "" {
		return domain.PlacementDecision{}, errors.New("regionId or required node is required")
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return domain.PlacementDecision{}, err
	}
	filtered, err := s.FilterNodes(ctx, req, nodes)
	if err != nil {
		return domain.PlacementDecision{}, err
	}
	if len(filtered) == 0 {
		return domain.PlacementDecision{}, errors.New("no nodes satisfy placement constraints")
	}
	scores, err := s.ScoreNodes(ctx, req, filtered)
	if err != nil {
		return domain.PlacementDecision{}, err
	}
	if len(scores) == 0 {
		return domain.PlacementDecision{}, errors.New("no nodes available for placement")
	}
	sort.SliceStable(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})
	selected := scores[0]
	regionID := req.RegionID
	if regionID == "" && selected.Node.RegionID != nil {
		regionID = *selected.Node.RegionID
	}
	return domain.PlacementDecision{
		RegionID:     regionID,
		RegionIDRaw:  regionID,
		NodeID:       selected.Node.ID,
		NodeIDRaw:    selected.Node.ID,
		AllocationID: req.AllocationID,
		Manual:       req.RequiredNode != "",
		Score:        selected.Score,
		Reasons:      []string{selected.Reason},
	}, nil
}

func (s *Scheduler) FilterNodes(ctx context.Context, req domain.PlacementRequest, nodes []store.Node) ([]store.Node, error) {
	req = normalizeRequest(req)
	// TODO: Wire in ConstraintScheduler.EvaluateConstraints to apply admin-defined scheduling constraints during filtering.
	regions, err := s.store.ListRegions(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]store.Node, 0, len(nodes))
	for _, node := range nodes {
		if !nodeRegionEnabled(node, regions) {
			s.recordPlacementRejection()
			continue
		}
		if req.RequiredNode != "" && node.ID != req.RequiredNode {
			continue
		}
		if node.ActualState != string(domain.NodeActualStateOnline) || node.DesiredState == store.NodeDesiredStateMaintenance || node.DesiredState == store.NodeDesiredStateDraining || node.Maintenance || node.Draining {
			s.recordPlacementRejection()
			continue
		}
		if req.RegionID != "" && (node.RegionID == nil || *node.RegionID != req.RegionID) {
			s.recordPlacementRejection()
			continue
		}
		snapshot, err := s.store.NodeCapacitySnapshot(ctx, node.ID)
		if err != nil {
			s.recordPlacementRejection()
			continue
		}
		if !hasCapacity(snapshot.TotalCPU, snapshot.AvailableCPU, req.CPU) {
			s.recordCapacityExceeded(ctx, node.ID, "cpu", snapshot.AvailableCPU, req.CPU)
			continue
		}
		if !hasCapacity(snapshot.TotalMemory, snapshot.AvailableMemory, req.MemoryMB) {
			s.recordCapacityExceeded(ctx, node.ID, "memory", snapshot.AvailableMemory, req.MemoryMB)
			continue
		}
		if !hasCapacity(snapshot.TotalDisk, snapshot.AvailableDisk, req.DiskMB) {
			s.recordCapacityExceeded(ctx, node.ID, "disk", snapshot.AvailableDisk, req.DiskMB)
			continue
		}
		filtered = append(filtered, node)
	}
	return filtered, nil
}

func (s *Scheduler) recordPlacementRejection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics.PlacementRejectionsTotal++
}

func (s *Scheduler) recordCapacityExceeded(ctx context.Context, nodeID, resource string, available, requested int) {
	s.mu.Lock()
	s.metrics.PlacementRejectionsTotal++
	s.metrics.CapacityExceededTotal++
	s.mu.Unlock()
	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope(events.EventNodeCapacityExceeded, "scheduler", "node", nodeID, map[string]any{
			"resource":  resource,
			"available": available,
			"requested": requested,
		}))
	}
}

func (s *Scheduler) ScoreNodes(ctx context.Context, req domain.PlacementRequest, nodes []store.Node) ([]NodeScore, error) {
	req = normalizeRequest(req)
	// TODO: Wire in PredictiveScorer to incorporate predictive scores (trend/affinity/anti-affinity) into the scoring pipeline.
	workload := toWorkloadRequest(req)

	allServers, err := s.store.ListServers(ctx)
	if err == nil {
		serverNodeMap := make(map[string]string, len(allServers))
		for _, sv := range allServers {
			serverNodeMap[sv.ID] = sv.Node
		}
		workload.ConstraintCtx = placement.ConstraintContext{
			ServerNodeMap: serverNodeMap,
		}
	}

	nodeMap := make(map[string]store.Node, len(nodes))
	candidates := make([]placement.Candidate, 0, len(nodes))
	for _, node := range nodes {
		snapshot, err := s.store.NodeCapacitySnapshot(ctx, node.ID)
		if err != nil {
			continue
		}
		nodeMap[node.ID] = node
		candidates = append(candidates, nodeToCandidate(snapshot, node))
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	results, err := s.engine.PlaceAll(ctx, candidates, workload)
	if err != nil {
		return nil, err
	}
	scores := make([]NodeScore, 0, len(results))
	for _, r := range results {
		node, ok := nodeMap[r.NodeID]
		if !ok {
			continue
		}
		reason := strings.Join(r.Reasons, "; ")
		if req.PreferredNode != "" && node.ID == req.PreferredNode {
			r.Score += 1e9
			reason = "preferred node"
		}
		scores = append(scores, NodeScore{Node: node, Score: r.Score, Reason: reason})
	}
	return scores, nil
}

func nodeToCandidate(snapshot store.NodeCapacitySnapshot, node store.Node) placement.Candidate {
	status := "online"
	if node.Maintenance || node.DesiredState == store.NodeDesiredStateMaintenance {
		status = "maintenance"
	} else if node.Draining || node.DesiredState == store.NodeDesiredStateDraining {
		status = "draining"
	}
	regionID := ""
	if node.RegionID != nil {
		regionID = *node.RegionID
	}
	return placement.Candidate{
		NodeID:          node.ID,
		RegionID:        regionID,
		TotalCPU:        snapshot.TotalCPU,
		TotalMemory:     snapshot.TotalMemory,
		TotalDisk:       snapshot.TotalDisk,
		AllocatedCPU:    snapshot.AllocatedCPU,
		AllocatedMemory: snapshot.AllocatedMemory,
		AllocatedDisk:   snapshot.AllocatedDisk,
		AvailableCPU:    snapshot.AvailableCPU,
		AvailableMemory: snapshot.AvailableMemory,
		AvailableDisk:   snapshot.AvailableDisk,
		ServerCount:     snapshot.ServerCount,
		Maintenance:     node.Maintenance,
		Draining:        node.Draining,
		Status:          status,
	}
}

func toWorkloadRequest(req domain.PlacementRequest) placement.WorkloadRequest {
	return placement.WorkloadRequest{
		CPU:           req.CPU,
		MemoryMB:      req.MemoryMB,
		DiskMB:        req.DiskMB,
		PreferredNode: req.PreferredNode,
		RequiredNode:  req.RequiredNode,
		RegionID:      req.RegionID,
	}
}

func normalizeRequest(req domain.PlacementRequest) domain.PlacementRequest {
	req.RegionID = strings.TrimSpace(firstNonEmpty(req.RegionID, req.Region))
	req.RequiredNode = strings.TrimSpace(firstNonEmpty(req.RequiredNode, req.NodeID))
	req.PreferredNode = strings.TrimSpace(req.PreferredNode)
	req.AllocationID = strings.TrimSpace(req.AllocationID)
	if req.CPU == 0 {
		req.CPU = req.CPUShares
	}
	if req.CPU == 0 {
		req.CPU = 1024
	}
	if req.MemoryMB == 0 {
		req.MemoryMB = 2048
	}
	if req.DiskMB == 0 {
		req.DiskMB = 10240
	}
	return req
}

func hasCapacity(total, available, requested int) bool {
	if requested <= 0 || total <= 0 {
		return true
	}
	return available >= requested
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Scheduler) resolveRegionID(ctx context.Context, value string) (string, error) {
	regions, err := s.store.ListRegions(ctx)
	if err != nil {
		return "", err
	}
	needle := strings.ToLower(strings.TrimSpace(value))
	for _, region := range regions {
		if strings.ToLower(region.ID) == needle || strings.ToLower(region.Slug) == needle || strings.ToLower(region.Name) == needle {
			return region.ID, nil
		}
	}
	return "", nil
}

func nodeRegionEnabled(node store.Node, regions []store.Region) bool {
	if node.RegionID == nil {
		return true
	}
	for _, region := range regions {
		if region.ID == *node.RegionID {
			return region.Enabled
		}
	}
	return false
}
