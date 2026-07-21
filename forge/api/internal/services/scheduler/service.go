package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/placement"
	"gamepanel/forge/internal/services/reservations"
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
	PlaceReplicas(context.Context, domain.PlaceReplicasRequest) ([]domain.PlacementReason, error)
	ScaleReplicas(context.Context, domain.ScaleRequest) ([]domain.PlacementReason, error)
	ReplaceFailedInstance(context.Context, domain.ReplaceFailedInstanceRequest) (*domain.PlacementReason, error)
}

type Scheduler struct {
	store               *store.Store
	engine              *placement.Engine
	publisher           events.Publisher
	predictiveScorer    *PredictiveScorer
	constraintScheduler *ConstraintScheduler
	reservations        *reservations.Manager
	mu                  sync.Mutex
	metrics             Metrics
}

type Metrics struct {
	PlacementRejectionsTotal uint64 `json:"placement_rejections_total"`
	CapacityExceededTotal    uint64 `json:"capacity_exceeded_total"`
	ReplicasPlacedTotal      uint64 `json:"replicas_placed_total"`
	ScaleUpTotal             uint64 `json:"scale_up_total"`
	ScaleDownTotal           uint64 `json:"scale_down_total"`
	FailedReplacementsTotal  uint64 `json:"failed_replacements_total"`
}

func New(store *store.Store, engine *placement.Engine, publishers ...events.Publisher) *Scheduler {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Scheduler{store: store, engine: engine, publisher: publisher}
}

func (s *Scheduler) WithPredictiveScorer(ps *PredictiveScorer) *Scheduler {
	s.predictiveScorer = ps
	return s
}

func (s *Scheduler) WithConstraintScheduler(cs *ConstraintScheduler) *Scheduler {
	s.constraintScheduler = cs
	return s
}

func (s *Scheduler) WithReservations(mgr *reservations.Manager) *Scheduler {
	s.reservations = mgr
	return s
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

	for _, scored := range scores {
		var reservation store.PlacementReservation
		var err error
		if !req.SkipReservation {
			if s.reservations != nil {
				reservation, err = s.reservations.CreateReservation(ctx, store.CreatePlacementReservationRequest{
					NodeID:          scored.Node.ID,
					ReservationType: store.PlacementReservationTypePlacement,
					CPU:             req.CPU,
					Memory:          int64(req.MemoryMB),
					Disk:            int64(req.DiskMB),
				})
			} else {
				reservation, err = s.store.CreatePlacementReservation(ctx, store.CreatePlacementReservationRequest{
					NodeID:          scored.Node.ID,
					ReservationType: store.PlacementReservationTypePlacement,
					CPU:             req.CPU,
					Memory:          int64(req.MemoryMB),
					Disk:            int64(req.DiskMB),
				})
			}
			if err != nil {
				continue
			}
		}
		regionID := req.RegionID
		if regionID == "" && scored.Node.RegionID != nil {
			regionID = *scored.Node.RegionID
		}
		return domain.PlacementDecision{
			RegionID:      regionID,
			RegionIDRaw:   regionID,
			NodeID:        scored.Node.ID,
			NodeIDRaw:     scored.Node.ID,
			AllocationID:  req.AllocationID,
			ReservationID: reservation.ID,
			Manual:        req.RequiredNode != "",
			Score:         scored.Score,
			Reasons:       []string{scored.Reason, "reserved capacity on placed node"},
		}, nil
	}
	return domain.PlacementDecision{}, errors.New("insufficient capacity on all candidate nodes")
}

func (s *Scheduler) FilterNodes(ctx context.Context, req domain.PlacementRequest, nodes []store.Node) ([]store.Node, error) {
	req = normalizeRequest(req)
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
		if req.StorageLocality == "local_only" && node.RuntimeProvider != "" && node.RuntimeProvider != "local" {
			s.recordPlacementRejection()
			continue
		}
		filtered = append(filtered, node)
	}
	if s.constraintScheduler != nil {
		filtered, err = s.constraintScheduler.EvaluateConstraints(ctx, req, filtered)
		if err != nil {
			return nil, err
		}
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
		if s.predictiveScorer != nil {
			ps, err := s.predictiveScorer.ScorePredictive(ctx, node.ID, req)
			if err == nil && ps != nil {
				r.Score = r.Score*(1+ps.TrendScore) + ps.AffinityScore - ps.AntiAffinityScore
				reason = reason + "; predictive: trend=" + fmt.Sprintf("%.4f", ps.TrendScore) + " affinity=" + fmt.Sprintf("%.4f", ps.AffinityScore) + " anti-affinity=" + fmt.Sprintf("%.4f", ps.AntiAffinityScore)
			}
		}
		if req.StorageLocality != "" && r.StorageLocality != req.StorageLocality {
			r.Score -= 1e10
			reason = reason + "; storage locality mismatch penalty"
		} else if req.StorageLocality != "" && r.StorageLocality == req.StorageLocality {
			r.Score += 1e8
			reason = reason + "; storage locality match bonus"
		}
		scores = append(scores, NodeScore{Node: node, Score: r.Score, Reason: reason})
	}
	return scores, nil
}

func (s *Scheduler) PlaceReplicas(ctx context.Context, req domain.PlaceReplicasRequest) ([]domain.PlacementReason, error) {
	if s.store == nil {
		return nil, errors.New("scheduler not initialized")
	}
	app, err := s.store.GetReplicaApp(ctx, req.AppID)
	if err != nil {
		return nil, fmt.Errorf("app not found: %w", err)
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	filtered, err := s.FilterNodes(ctx, domain.PlacementRequest{RegionID: req.RegionID, CPU: req.CPU, MemoryMB: req.MemoryMB, DiskMB: req.DiskMB, RequiredNode: req.RequiredNode}, nodes)
	if err != nil {
		return nil, err
	}
	if req.RuntimeFilter != "" {
		filtered = filterByRuntimeProvider(filtered, req.RuntimeFilter)
	}
	existing, err := s.store.ListInstancesByApp(ctx, req.AppID)
	if err != nil {
		return nil, err
	}
	existingNodeMap := make(map[string]int)
	for _, inst := range existing {
		if inst.Status != "removing" && inst.Status != "failed" {
			existingNodeMap[inst.NodeID]++
		}
	}

	candidates := make([]placement.Candidate, 0, len(filtered))
	for _, node := range filtered {
		snapshot, err := s.store.NodeCapacitySnapshot(ctx, node.ID)
		if err != nil {
			continue
		}
		candidates = append(candidates, nodeToCandidate(snapshot, node))
	}

	replicas := make([]placement.ReplicaSpec, req.ReplicaCount)
	for i := 0; i < req.ReplicaCount; i++ {
		runtime := app.RuntimeProvider
		if req.RuntimeFilter != "" {
			runtime = req.RuntimeFilter
		}
		replicas[i] = placement.ReplicaSpec{
			Index:           i,
			CPU:             req.CPU,
			MemoryMB:        req.MemoryMB,
			DiskMB:          req.DiskMB,
			RuntimeProvider: runtime,
		}
	}

	placementReq := placement.ReplicaPlacementRequest{
		AppID:           req.AppID,
		Replicas:        replicas,
		RegionID:        req.RegionID,
		RequiredNode:    req.RequiredNode,
		PreferredNode:   req.PreferredNode,
		RuntimeFilter:   req.RuntimeFilter,
		ExistingNodeMap: existingNodeMap,
	}

	result, err := s.engine.PlaceReplicas(ctx, candidates, placementReq)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.metrics.ReplicasPlacedTotal += uint64(len(result.Placements))
	s.mu.Unlock()

	reasons := make([]domain.PlacementReason, 0, len(result.Placements))
	for _, p := range result.Placements {
		reasons = append(reasons, domain.PlacementReason{
			InstanceID: "",
			NodeID:     p.NodeID,
			Index:      p.Index,
			Score:      p.Score,
			Accepted:   true,
			Reasons:    p.Reasons,
		})
	}
	for _, f := range result.Failures {
		reasons = append(reasons, domain.PlacementReason{
			InstanceID: "",
			NodeID:     "",
			Index:      f.Index,
			Score:      0,
			Accepted:   false,
			Reasons:    []string{f.Reason},
		})
	}
	return reasons, nil
}

func (s *Scheduler) ScaleReplicas(ctx context.Context, req domain.ScaleRequest) ([]domain.PlacementReason, error) {
	if s.store == nil {
		return nil, errors.New("scheduler not initialized")
	}
	app, err := s.store.GetReplicaApp(ctx, req.AppID)
	if err != nil {
		return nil, fmt.Errorf("app not found: %w", err)
	}
	current, err := s.store.ListInstancesByApp(ctx, req.AppID)
	if err != nil {
		return nil, err
	}
	activeInstances := 0
	for _, inst := range current {
		if inst.Status != "removing" && inst.Status != "failed" {
			activeInstances++
		}
	}

	if req.ReplicaCount == activeInstances {
		return nil, nil
	}

	if req.ReplicaCount > activeInstances {
		// Scale up
		s.mu.Lock()
		s.metrics.ScaleUpTotal++
		s.mu.Unlock()

		extra := req.ReplicaCount - activeInstances
		allNodes, err := s.store.ListNodes(ctx)
		if err != nil {
			return nil, err
		}
		filtered, err := s.FilterNodes(ctx, domain.PlacementRequest{RegionID: "", CPU: app.CPU, MemoryMB: app.MemoryMB, DiskMB: app.DiskMB}, allNodes)
		if err != nil {
			return nil, err
		}

		existingNodeMap := make(map[string]int)
		for _, inst := range current {
			if inst.Status != "removing" && inst.Status != "failed" {
				existingNodeMap[inst.NodeID]++
			}
		}

		candidates := make([]placement.Candidate, 0, len(filtered))
		for _, node := range filtered {
			snapshot, err := s.store.NodeCapacitySnapshot(ctx, node.ID)
			if err != nil {
				continue
			}
			candidates = append(candidates, nodeToCandidate(snapshot, node))
		}

		startIdx := activeInstances
		replicas := make([]placement.ReplicaSpec, extra)
		for i := 0; i < extra; i++ {
			replicas[i] = placement.ReplicaSpec{
				Index:           startIdx + i,
				CPU:             app.CPU,
				MemoryMB:        app.MemoryMB,
				DiskMB:          app.DiskMB,
				RuntimeProvider: app.RuntimeProvider,
			}
		}

		placementReq := placement.ReplicaPlacementRequest{
			AppID:           req.AppID,
			Replicas:        replicas,
			ExistingNodeMap: existingNodeMap,
		}
		result, err := s.engine.PlaceReplicas(ctx, candidates, placementReq)
		if err != nil {
			return nil, err
		}
		reasons := make([]domain.PlacementReason, 0, len(result.Placements))
		for _, p := range result.Placements {
			reasons = append(reasons, domain.PlacementReason{
				NodeID:   p.NodeID,
				Index:    p.Index,
				Score:    p.Score,
				Accepted: true,
				Reasons:  p.Reasons,
			})
		}
		for _, f := range result.Failures {
			reasons = append(reasons, domain.PlacementReason{
				Index:    f.Index,
				Accepted: false,
				Reasons:  []string{f.Reason},
			})
		}
		_, _ = s.store.UpdateReplicaAppReplicas(ctx, req.AppID, req.ReplicaCount)
		return reasons, nil
	}

	// Scale down
	s.mu.Lock()
	s.metrics.ScaleDownTotal++
	s.mu.Unlock()

	remove := activeInstances - req.ReplicaCount
	toRemove := make([]store.Instance, 0, remove)
	for i := len(current) - 1; i >= 0 && len(toRemove) < remove; i-- {
		if current[i].Status != "removing" && current[i].Status != "failed" {
			toRemove = append(toRemove, current[i])
		}
	}
	reasons := make([]domain.PlacementReason, 0, len(toRemove))
	for _, inst := range toRemove {
		_, _ = s.store.UpdateInstanceStatus(ctx, inst.ID, "removing")
		reasons = append(reasons, domain.PlacementReason{
			InstanceID: inst.ID,
			NodeID:     inst.NodeID,
			Accepted:   true,
			Reasons:    []string{"scale down - removed instance"},
		})
	}
	_, _ = s.store.UpdateReplicaAppReplicas(ctx, req.AppID, req.ReplicaCount)
	return reasons, nil
}

func (s *Scheduler) ReplaceFailedInstance(ctx context.Context, req domain.ReplaceFailedInstanceRequest) (*domain.PlacementReason, error) {
	if s.store == nil {
		return nil, errors.New("scheduler not initialized")
	}
	inst, err := s.store.GetInstance(ctx, req.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("instance not found: %w", err)
	}
	app, err := s.store.GetReplicaApp(ctx, inst.AppID)
	if err != nil {
		return nil, err
	}

	// Mark current as removing
	_, _ = s.store.UpdateInstanceStatus(ctx, inst.ID, "removing")

	// Find replacement node
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	filtered, filterErr := s.FilterNodes(ctx, domain.PlacementRequest{CPU: app.CPU, MemoryMB: app.MemoryMB, DiskMB: app.DiskMB}, nodes)
	if filterErr != nil {
		slog.WarnContext(ctx, "node filtering failed during replacement, falling back to all nodes", "error", filterErr)
		filtered = nodes
	} else if filtered == nil {
		filtered = nodes
	}

	existing, _ := s.store.ListInstancesByApp(ctx, app.ID)
	existingNodeMap := make(map[string]int)
	for _, e := range existing {
		if e.Status != "removing" && e.Status != "failed" {
			existingNodeMap[e.NodeID]++
		}
	}

	candidates := make([]placement.Candidate, 0, len(filtered))
	for _, node := range filtered {
		snapshot, err := s.store.NodeCapacitySnapshot(ctx, node.ID)
		if err != nil {
			continue
		}
		candidates = append(candidates, nodeToCandidate(snapshot, node))
	}

	replicas := []placement.ReplicaSpec{{
		Index:           inst.Idx,
		CPU:             app.CPU,
		MemoryMB:        app.MemoryMB,
		DiskMB:          app.DiskMB,
		RuntimeProvider: app.RuntimeProvider,
	}}

	placementReq := placement.ReplicaPlacementRequest{
		AppID:           app.ID,
		Replicas:        replicas,
		ExistingNodeMap: existingNodeMap,
	}
	result, err := s.engine.PlaceReplicas(ctx, candidates, placementReq)
	if err != nil {
		return nil, err
	}

	if len(result.Placements) == 0 {
		// Restore instance
		_, _ = s.store.UpdateInstanceStatus(ctx, inst.ID, "failed")
		s.mu.Lock()
		s.metrics.FailedReplacementsTotal++
		s.mu.Unlock()
		return &domain.PlacementReason{
			InstanceID: inst.ID,
			Accepted:   false,
			Reasons:    []string{"no replacement node found"},
		}, nil
	}

	p := result.Placements[0]
	// Update instance to new node
	_, _ = s.store.UpdateInstanceNode(ctx, inst.ID, p.NodeID)
	_, _ = s.store.UpdateInstanceStatus(ctx, inst.ID, "pending")

	return &domain.PlacementReason{
		InstanceID: inst.ID,
		NodeID:     p.NodeID,
		Score:      p.Score,
		Accepted:   true,
		Reasons:    append(p.Reasons, "replaced failed instance"),
	}, nil
}

func filterByRuntimeProvider(nodes []store.Node, runtime string) []store.Node {
	var filtered []store.Node
	for _, n := range nodes {
		if n.RuntimeProvider == "" || n.RuntimeProvider == runtime || runtime == "" {
			filtered = append(filtered, n)
		}
	}
	if len(filtered) == 0 {
		return nodes
	}
	return filtered
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
	storageLocality := "local"
	if node.RuntimeProvider == "nfs" || node.RuntimeProvider == "shared" {
		storageLocality = "shared"
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
		StorageLocality: storageLocality,
	}
}

func toWorkloadRequest(req domain.PlacementRequest) placement.WorkloadRequest {
	return placement.WorkloadRequest{
		CPU:             req.CPU,
		MemoryMB:        req.MemoryMB,
		DiskMB:          req.DiskMB,
		PreferredNode:   req.PreferredNode,
		RequiredNode:    req.RequiredNode,
		RegionID:        req.RegionID,
		StorageLocality: req.StorageLocality,
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
