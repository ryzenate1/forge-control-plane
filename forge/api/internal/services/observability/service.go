package observability

import (
	"context"
	"strings"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type Service struct {
	store   *store.Store
	metrics *MetricsHistory
}

func New(store *store.Store) *Service {
	return &Service{store: store, metrics: NewMetricsHistory(60)}
}

func (s *Service) StartMetricsCollection(ctx context.Context, interval time.Duration) {
	s.metrics.StartCollection(ctx, interval)
}

func (s *Service) Handle(ctx context.Context, event events.Envelope) error {
	if s == nil || s.store == nil {
		return nil
	}
	_, err := s.store.CreateTimelineEvent(ctx, store.CreateTimelineEventRequest{
		EventID:       event.ID,
		ResourceType:  event.ResourceType,
		ResourceID:    event.ResourceID,
		EventType:     string(event.Type),
		CorrelationID: event.CorrelationID,
		Source:        event.Source,
		Timestamp:     event.Timestamp,
		Payload:       event.Payload,
	})
	return err
}

func (s *Service) Timeline(ctx context.Context, limit int) ([]store.TimelineEvent, error) {
	return s.store.ListTimelineEvents(ctx, store.TimelineQuery{Limit: limit})
}

func (s *Service) ResourceTimeline(ctx context.Context, resourceType, resourceID string, limit int) ([]store.TimelineEvent, error) {
	return s.store.ListTimelineEvents(ctx, store.TimelineQuery{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Limit:        limit,
	})
}

func (s *Service) Correlation(ctx context.Context, correlationID string, limit int) ([]store.TimelineEvent, error) {
	return s.store.ListTimelineEvents(ctx, store.TimelineQuery{
		CorrelationID: correlationID,
		Limit:         limit,
	})
}

func (s *Service) NodeHeartbeatHistory(ctx context.Context, nodeID string, limit int) ([]store.NodeHeartbeatHistory, error) {
	return s.store.ListNodeHeartbeatHistory(ctx, nodeID, limit)
}

func (s *Service) NodeHealthHistory(ctx context.Context, nodeID string, limit int) ([]store.NodeHealthHistory, error) {
	return s.store.ListNodeHealthHistory(ctx, nodeID, limit)
}

func (s *Service) RecordNodeHeartbeat(ctx context.Context, node store.Node, req store.NodeHeartbeatRequest) {
	if s == nil || s.store == nil || strings.TrimSpace(node.ID) == "" {
		return
	}
	success := strings.TrimSpace(req.Error) == ""
	if _, err := s.store.CreateNodeHeartbeatHistory(ctx, store.CreateNodeHeartbeatHistoryRequest{
		NodeID:        node.ID,
		Success:       success,
		FailureReason: req.Error,
		Version:       req.Version,
		OS:            req.OS,
		Architecture:  req.Architecture,
		CPUThreads:    req.CPUThreads,
		MemoryMB:      req.MemoryMB,
		DiskMB:        req.DiskMB,
		RuntimeStatus: req.DockerStatus,
	}); err != nil {
		return
	}
}

func (s *Service) RecordNodeHealth(ctx context.Context, node store.Node) {
	if s == nil || s.store == nil || strings.TrimSpace(node.ID) == "" {
		return
	}
	s.recordNodeHealth(ctx, node)
}

func (s *Service) RecordHeartbeatFailure(ctx context.Context, nodeID, reason string) {
	if s == nil || s.store == nil || strings.TrimSpace(nodeID) == "" {
		return
	}
	_, _ = s.store.CreateNodeHeartbeatHistory(ctx, store.CreateNodeHeartbeatHistoryRequest{
		NodeID:        nodeID,
		Success:       false,
		FailureReason: reason,
	})
}

// Node Metrics
func (s *Service) RecordNodeMetric(ctx context.Context, req store.CreateNodeMetricRequest) (store.NodeMetric, error) {
	return s.store.CreateNodeMetric(ctx, req)
}

func (s *Service) ListNodeMetrics(ctx context.Context, nodeID string, limit int, since *time.Time) ([]store.NodeMetric, error) {
	return s.store.ListNodeMetrics(ctx, nodeID, limit, since)
}

func (s *Service) ListAllNodeMetricsLatest(ctx context.Context) ([]store.NodeMetric, error) {
	return s.store.ListNodeMetricsLatest(ctx)
}

// Workload Metrics
func (s *Service) RecordWorkloadMetric(ctx context.Context, req store.CreateWorkloadMetricRequest) (store.WorkloadMetric, error) {
	return s.store.CreateWorkloadMetric(ctx, req)
}

func (s *Service) ListWorkloadMetrics(ctx context.Context, serverID string, limit int) ([]store.WorkloadMetric, error) {
	return s.store.ListWorkloadMetrics(ctx, serverID, limit)
}

// Build Logs
func (s *Service) RecordBuildLog(ctx context.Context, req store.CreateBuildLogEntryRequest) (store.BuildLogEntry, error) {
	return s.store.CreateBuildLog(ctx, req)
}

func (s *Service) BuildLogs(ctx context.Context, buildID string, limit int) ([]store.BuildLogEntry, error) {
	return s.store.ListBuildLogs(ctx, buildID, limit)
}

func (s *Service) BuildLogsByCorrelation(ctx context.Context, correlationID string, limit int) ([]store.BuildLogEntry, error) {
	return s.store.ListBuildLogsByCorrelation(ctx, correlationID, limit)
}

// Deployment Logs
func (s *Service) RecordDeploymentLog(ctx context.Context, req store.CreateDeploymentLogEntryRequest) (store.DeploymentLogEntry, error) {
	return s.store.CreateDeploymentLog(ctx, req)
}

func (s *Service) DeploymentLogs(ctx context.Context, deploymentID string, limit int) ([]store.DeploymentLogEntry, error) {
	return s.store.ListDeploymentLogs(ctx, deploymentID, limit)
}

// Beacon Command Logs
func (s *Service) RecordBeaconCommandLog(ctx context.Context, req store.CreateBeaconCommandLogRequest) (store.BeaconCommandLog, error) {
	return s.store.CreateBeaconCommandLog(ctx, req)
}

func (s *Service) BeaconCommandLogs(ctx context.Context, filter store.BeaconCommandLogFilter) ([]store.BeaconCommandLog, error) {
	return s.store.ListBeaconCommandLogs(ctx, filter)
}

func (s *Service) UpdateBeaconCommandStatus(ctx context.Context, commandID, status string, exitCode int, responsePayload map[string]any) error {
	return s.store.UpdateBeaconCommandLogStatus(ctx, commandID, status, exitCode, responsePayload)
}

// Correlation Links
func (s *Service) CreateCorrelationLink(ctx context.Context, req store.CreateCorrelationLinkRequest) (store.CorrelationLink, error) {
	return s.store.CreateCorrelationLink(ctx, req)
}

func (s *Service) CorrelationLinks(ctx context.Context, operationID string) ([]store.CorrelationLink, error) {
	return s.store.GetCorrelationLinks(ctx, operationID)
}

// Health History
func (s *Service) RecordHealthCheck(ctx context.Context, rec store.HealthHistoryRecord) error {
	return s.store.CreateHealthHistory(ctx, rec)
}

func (s *Service) HealthHistory(ctx context.Context, checkName string, limit int) ([]store.HealthHistoryRecord, error) {
	return s.store.ListHealthHistory(ctx, checkName, limit)
}

func (s *Service) AllHealthHistory(ctx context.Context, limit int) ([]store.HealthHistoryRecord, error) {
	return s.store.ListAllHealthHistory(ctx, limit)
}

// Retention
func (s *Service) EnforceRetention(ctx context.Context) (map[string]int64, error) {
	return s.store.EnforceRetention(ctx)
}

func (s *Service) RetentionPolicies(ctx context.Context) ([]store.RetentionPolicy, error) {
	return s.store.ListRetentionPolicies(ctx)
}

func (s *Service) UpdateRetentionPolicy(ctx context.Context, metricType string, ttlHours, maxRecords int) error {
	return s.store.UpdateRetentionPolicy(ctx, metricType, ttlHours, maxRecords)
}

// Dashboard / Monitoring summary
func (s *Service) MonitoringSummary(ctx context.Context) (map[string]any, error) {
	summary := map[string]any{}

	latestMetrics, err := s.store.ListNodeMetricsLatest(ctx)
	if err == nil {
		summary["nodes"] = latestMetrics
	}

	alertCount, err := s.store.CountAlerts(ctx, store.AlertFilter{Acknowledged: boolPtr(false), Limit: 1})
	if err == nil {
		summary["unacknowledgedAlerts"] = alertCount
	}

	healthHistory, err := s.store.ListAllHealthHistory(ctx, 50)
	if err == nil {
		summary["recentHealthChecks"] = healthHistory
	}

	return summary, nil
}

func (s *Service) recordNodeHealth(ctx context.Context, node store.Node) {
	capacity, err := s.store.NodeCapacitySnapshot(ctx, node.ID)
	if err != nil {
		return
	}
	cpu := resourceScore(capacity.TotalCPU, capacity.AvailableCPU)
	memory := resourceScore(capacity.TotalMemory, capacity.AvailableMemory)
	disk := resourceScore(capacity.TotalDisk, capacity.AvailableDisk)
	heartbeat := heartbeatScore(node.LastSeenAt)
	status := statusScore(node.ActualState)
	total := (cpu + memory + disk + heartbeat + status) / 5
	_, _ = s.store.CreateNodeHealthHistory(ctx, store.CreateNodeHealthHistoryRequest{
		NodeID:          node.ID,
		ActualState:     node.ActualState,
		DesiredState:    string(node.DesiredState),
		HealthScore:     float64(total),
		CPUScore:        float64(cpu),
		MemoryScore:     float64(memory),
		DiskScore:       float64(disk),
		HeartbeatScore:  float64(heartbeat),
		StatusScore:     float64(status),
		AllocatedCPU:    capacity.AllocatedCPU,
		AvailableCPU:    capacity.AvailableCPU,
		AllocatedMemory: int64(capacity.AllocatedMemory),
		AvailableMemory: int64(capacity.AvailableMemory),
		AllocatedDisk:   int64(capacity.AllocatedDisk),
		AvailableDisk:   int64(capacity.AvailableDisk),
		ServerCount:     capacity.ServerCount,
	})
}

func resourceScore(total, available int) int {
	if total <= 0 {
		return 50
	}
	used := total - available
	if used < 0 {
		used = 0
	}
	score := 100 - ((used * 100) / total)
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func heartbeatScore(lastSeen *time.Time) int {
	if lastSeen == nil {
		return 0
	}
	age := time.Since(*lastSeen)
	switch {
	case age <= 2*time.Minute:
		return 100
	case age <= 5*time.Minute:
		return 75
	case age <= 15*time.Minute:
		return 40
	default:
		return 0
	}
}

func statusScore(status string) int {
	switch status {
	case "online":
		return 100
	case "degraded":
		return 40
	default:
		return 0
	}
}

func boolPtr(b bool) *bool {
	return &b
}
