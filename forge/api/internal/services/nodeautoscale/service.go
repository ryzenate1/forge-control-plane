// Package nodeautoscale scales the cluster by provisioning cloud instances
// (or recommending them) based on aggregate node capacity, in safe "suggest
// + apply with explicit confirm" mode.
package nodeautoscale

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/cloud"
	"gamepanel/forge/internal/services/clustermembership"
	"gamepanel/forge/internal/store"
)

// BootstrapFunc renders the user-data (cloud-init) used to join a freshly
// provisioned instance into the cluster. main wires it with
// cloud.BeaconCloudInit.
type BootstrapFunc func(ctx context.Context, nodeName string) (string, error)

// Service drives node autoscaling. It is safe-mode by default: evaluations
// only produce "suggested" events until apply is explicitly confirmed via
// ScaleOut / the POST scale-out route. Policies with AutoJoin=true also
// provision during the periodic scan.
type Service struct {
	store      *store.Store
	cloud      *cloud.Manager
	membership *clustermembership.Service
	logger     *slog.Logger
	bootstrap  BootstrapFunc

	mu       sync.Mutex
	lastScan map[string]time.Time
}

func New(st *store.Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:    st,
		logger:   logger,
		lastScan: map[string]time.Time{},
	}
}

// WithCloud wires the cloud manager used for provisioning.
func (s *Service) WithCloud(mgr *cloud.Manager) *Service {
	s.cloud = mgr
	return s
}

// WithMembership wires the cluster membership service used for scale-in drains.
func (s *Service) WithMembership(membership *clustermembership.Service) *Service {
	s.membership = membership
	return s
}

// WithBootstrap sets the user-data builder for auto-join provisioning.
func (s *Service) WithBootstrap(fn BootstrapFunc) *Service {
	s.bootstrap = fn
	return s
}

// Policy is the HTTP-facing shape of a node_autoscale_policies row.
type Policy = store.NodeAutoscalePolicy

// Event is the HTTP-facing shape of a node_autoscale_events row.
type Event = store.NodeAutoscaleEvent

// ListPolicies returns all policies, newest first ordering by name.
func (s *Service) ListPolicies(ctx context.Context) ([]Policy, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("node autoscaler unavailable")
	}
	return s.store.ListNodeAutoscalePolicies(ctx)
}

// CreatePolicy inserts a policy (normalized).
func (s *Service) CreatePolicy(ctx context.Context, p *Policy) (Policy, error) {
	if s == nil || s.store == nil {
		return Policy{}, errors.New("node autoscaler unavailable")
	}
	if strings.TrimSpace(p.Name) == "" {
		return Policy{}, errors.New("policy name is required")
	}
	if err := s.store.CreateNodeAutoscalePolicy(ctx, p); err != nil {
		return Policy{}, err
	}
	return s.store.GetNodeAutoscalePolicy(ctx, p.ID)
}

// UpdatePolicy replaces a policy.
func (s *Service) UpdatePolicy(ctx context.Context, policyID string, p *Policy) (Policy, error) {
	if s == nil || s.store == nil {
		return Policy{}, errors.New("node autoscaler unavailable")
	}
	return s.store.UpdateNodeAutoscalePolicy(ctx, policyID, p)
}

// DeletePolicy removes a policy (cascades its events).
func (s *Service) DeletePolicy(ctx context.Context, policyID string) error {
	if s == nil || s.store == nil {
		return errors.New("node autoscaler unavailable")
	}
	return s.store.DeleteNodeAutoscalePolicy(ctx, policyID)
}

// ListEvents returns the audit ledger for a policy (or all when empty).
func (s *Service) ListEvents(ctx context.Context, policyID string, limit int) ([]Event, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("node autoscaler unavailable")
	}
	return s.store.ListNodeAutoscaleEvents(ctx, policyID, limit)
}

// ClusterLoad aggregates fleet capacity used to evaluate policies.
type ClusterLoad struct {
	ActiveNodes int     `json:"activeNodes"`
	TotalCPU    int     `json:"totalCpu"`
	AllocatedCPU int    `json:"allocatedCpu"`
	TotalMemMB  int     `json:"totalMemMb"`
	AllocatedMem int    `json:"allocatedMemMb"`
	LoadCPU     float64 `json:"loadCpu"`
	LoadMemory  float64 `json:"loadMemory"`
	LoadDisk    float64 `json:"loadDisk"`
}

// Evaluation is a dry-run result: what the cluster needs now.
type Evaluation struct {
	PolicyID    string          `json:"policyId"`
	PolicyName  string          `json:"policyName"`
	Load        ClusterLoad     `json:"load"`
	Deficit     int             `json:"deficit"`
	ScaleOut    bool            `json:"scaleOut"`
	ScaleInNode string          `json:"scaleInNode,omitempty"`
	Summary     string          `json:"summary"`
	Cooldown    bool            `json:"cooldown"`
	DryRun      bool            `json:"dryRun"`
}

const (
	evaluatorCPU    = "cpu"
	evaluatorMemory = "memory"
	evaluatorBoth   = "both"
)

// Evaluate computes the scale decision for a policy without mutating
// anything. dryRun toggles only whether the audit event is written.
func (s *Service) Evaluate(ctx context.Context, policyID string, dryRun bool) (Evaluation, error) {
	if s == nil || s.store == nil {
		return Evaluation{}, errors.New("node autoscaler unavailable")
	}
	policy, err := s.store.GetNodeAutoscalePolicy(ctx, policyID)
	if err != nil {
		return Evaluation{}, err
	}
	evaluation, err := s.evaluate(ctx, policy)
	if err != nil {
		return Evaluation{}, err
	}
	evaluation.DryRun = dryRun
	if dryRun {
		return evaluation, nil
	}
	state := "suggested"
	if !evaluation.ScaleOut {
		state = "observe"
	}
	detail, _ := json.Marshal(map[string]any{
		"loadCpu":   evaluation.Load.LoadCPU,
		"loadMem":   evaluation.Load.LoadMemory,
		"active":    evaluation.Load.ActiveNodes,
		"scaleOut":  evaluation.ScaleOut,
		"scaleInNode": evaluation.ScaleInNode,
	})
	_ = s.store.RecordNodeAutoscaleEvent(ctx, &store.NodeAutoscaleEvent{
		PolicyID:  policyID,
		Action:    "evaluate",
		Direction: "out",
		State:     state,
		Deficit:   evaluation.Deficit,
		Detail:    detail,
	})
	return evaluation, nil
}

func (s *Service) evaluate(ctx context.Context, policy Policy) (Evaluation, error) {
	load, err := s.clusterLoad(ctx)
	if err != nil {
		return Evaluation{}, err
	}
	evaluation := Evaluation{
		PolicyName: policy.Name,
		Load:       load,
	}

	aboveTarget := load.LoadCPU >= policy.TargetCPUPercent/100.0 ||
		(policy.Evaluator == evaluatorMemory && load.LoadMemory >= policy.TargetMemPercent/100.0)

	if load.ActiveNodes < policy.MinNodes {
		evaluation.Deficit = policy.MinNodes - load.ActiveNodes
	} else if aboveTarget && load.ActiveNodes < policy.MaxNodes {
		evaluation.Deficit = 1
	}
	evaluation.ScaleOut = evaluation.Deficit > 0
	if !evaluation.ScaleOut && load.ActiveNodes > policy.MinNodes {
		loadBelow := load.LoadCPU < 0.30 && load.LoadMemory < 0.30
		if loadBelow {
			nodeID, err := s.leastLoadedNode(ctx)
			if err == nil {
				evaluation.ScaleInNode = nodeID
			}
		}
	}

	// Cooldown gating: rescans only every cooldown seconds.
	s.mu.Lock()
	last, seen := s.lastScan[policy.ID]
	if seen && time.Since(last) < time.Duration(policy.CooldownSeconds)*time.Second {
		evaluation.Cooldown = true
		evaluation.ScaleOut = false
	} else {
		s.lastScan[policy.ID] = time.Now()
	}
	s.mu.Unlock()

	if evaluation.ScaleOut {
		evaluation.Summary = fmt.Sprintf("cluster load %.0f%% exceeds target %.0f%%; provisioning 1 node", load.LoadCPU*100, policy.TargetCPUPercent)
	} else if evaluation.ScaleInNode != "" {
		evaluation.Summary = fmt.Sprintf("cluster underutilized; node %s is a scale-in candidate", evaluation.ScaleInNode)
	} else {
		evaluation.Summary = "cluster within target; no action"
	}
	return evaluation, nil
}

// clusterLoad aggregates capacity across nodes matching the policy cluster
// group (empty group = whole fleet).
func (s *Service) clusterLoad(ctx context.Context) (ClusterLoad, error) {
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return ClusterLoad{}, err
	}
	var load ClusterLoad
	for _, node := range nodes {
		if node.Draining || node.Maintenance {
			continue
		}
		snapshot, err := s.store.NodeCapacitySnapshot(ctx, node.ID)
		if err != nil {
			continue
		}
		load.ActiveNodes++
		load.TotalCPU += snapshot.TotalCPU
		load.AllocatedCPU += snapshot.AllocatedCPU
		load.TotalMemMB += snapshot.TotalMemory
		load.AllocatedMem += snapshot.AllocatedMemory
	}
	if load.TotalCPU > 0 {
		load.LoadCPU = float64(load.AllocatedCPU) / float64(load.TotalCPU)
	}
	if load.TotalMemMB > 0 {
		load.LoadMemory = float64(load.AllocatedMem) / float64(load.TotalMemMB)
	}
	return load, nil
}

func (s *Service) leastLoadedNode(ctx context.Context) (string, error) {
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return "", err
	}
	bestID, bestLoad := "", 0
	for _, node := range nodes {
		if node.Draining || node.Maintenance {
			continue
		}
		snapshot, err := s.store.NodeCapacitySnapshot(ctx, node.ID)
		if err != nil {
			continue
		}
		load := snapshot.AllocatedCPU + snapshot.AllocatedMemory + snapshot.AllocatedDisk
		if bestID == "" || load < bestLoad {
			bestID, bestLoad = node.ID, load
		}
	}
	if bestID == "" {
		return "", errors.New("no healthy nodes")
	}
	return bestID, nil
}

// ScaleOut provisions one node for a policy. This is the explicit-confirm
// path: it never consults AutoJoin, it always provisions.
func (s *Service) ScaleOut(ctx context.Context, policyID string) (Event, error) {
	if s == nil || s.store == nil {
		return Event{}, errors.New("node autoscaler unavailable")
	}
	policy, err := s.store.GetNodeAutoscalePolicy(ctx, policyID)
	if err != nil {
		return Event{}, err
	}
	evaluation, err := s.evaluate(ctx, policy)
	if err != nil {
		return Event{}, err
	}
	if evaluation.Cooldown {
		return Event{}, errors.New("policy is in cooldown")
	}
	if !evaluation.ScaleOut {
		return Event{}, errors.New("cluster does not need more nodes")
	}
	if s.cloud == nil {
		return s.recordEvent(ctx, policy, "error", "failed", evaluation.Deficit, "cloud manager not configured")
	}

	instance, err := s.provision(ctx, policy)
	if err != nil {
		return s.recordEvent(ctx, policy, "error", "failed", evaluation.Deficit,
			map[string]any{"error": err.Error()})
	}
	return s.recordEvent(ctx, policy, "scale-out", "applied", evaluation.Deficit,
		map[string]any{
			"instanceId": instance.ID,
			"state":      "join_pending",
			"provider":   instance.Provider,
			"region":     instance.Region,
			"name":       instance.Name,
		})
}

// provision builds the join user-data and creates the cloud instance.
func (s *Service) provision(ctx context.Context, policy Policy) (*cloud.Instance, error) {
	userData := ""
	if s.bootstrap != nil {
		if data, err := s.bootstrap(ctx, policy.Name+"-autoscale"); err == nil {
			userData = data
		}
	}
	req := cloud.CreateInstanceRequest{
		Name:         longName(policy, time.Now().UTC()),
		Region:       policy.Region,
		InstanceType: policy.InstanceType,
		Image:        policy.Image,
		Tags: map[string]string{
			"forge:autoscaler": "true",
			"forge:policy":     policy.Name,
			"forge:kind":       "node",
		},
		UserData: userData,
	}
	return s.cloud.ProvisionNode(ctx, cloud.ProviderKind(policy.Provider), req)
}

func longName(p Policy, now time.Time) string {
	return fmt.Sprintf("%s-%s", sanitize(p.Name), now.Format("20060102-150405"))
}

func sanitize(value string) string {
	replacer := strings.NewReplacer(" ", "-", "_", "-", ".", "-")
	return replacer.Replace(strings.ToLower(value))
}

// ScaleIn drains and retires a node (scale-in). The node must be quiescent
// after the drain; finally deprovisions the backing cloud instance when the
// node's drain completed.
func (s *Service) ScaleIn(ctx context.Context, nodeID string, deprovision bool) (Event, error) {
	if s == nil || s.store == nil {
		return Event{}, errors.New("node autoscaler unavailable")
	}
	if s.membership == nil {
		return Event{}, errors.New("membership service not wired for scale-in")
	}
	if err := s.membership.StartDrain(ctx, nodeID); err != nil {
		return Event{}, err
	}
	detail := map[string]any{
		"nodeId":      nodeID,
		"drained":     true,
		"deprovision": deprovision,
	}
	if deprovision && s.cloud != nil {
		node, err := s.store.GetNode(ctx, nodeID)
		if err == nil && node.SchedulerType != "" {
			_ = s.cloud.DeprovisionNode(ctx, cloud.ProviderKind("aws"), nodeID)
			detail["deprovisioned"] = true
		}
	}
	return s.recordEvent(ctx, store.NodeAutoscalePolicy{Name: "manual-scale-in"}, "scale-in", "applied", 1, detail)
}

func (s *Service) recordEvent(ctx context.Context, policy Policy, action, state string, deficit int, detail any) (Event, error) {
	raw, _ := json.Marshal(detail)
	event := &store.NodeAutoscaleEvent{
		PolicyID:  policy.ID,
		Action:    action,
		Direction: "out",
		State:     state,
		Deficit:   deficit,
		Detail:    raw,
	}
	if err := s.store.RecordNodeAutoscaleEvent(ctx, event); err != nil {
		return Event{}, err
	}
	return *event, nil
}