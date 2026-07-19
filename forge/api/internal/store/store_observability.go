package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Store) CreateTimelineEvent(ctx context.Context, req CreateTimelineEventRequest) (TimelineEvent, error) {
	if req.Timestamp.IsZero() {
		req.Timestamp = time.Now().UTC()
	}
	if req.CorrelationID == "" {
		req.CorrelationID = req.EventID
	}
	if req.CorrelationID == "" {
		req.CorrelationID = uuid.NewString()
	}
	if req.Source == "" {
		req.Source = "api"
	}
	payload := req.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return TimelineEvent{}, err
	}
	id := uuid.NewString()
	var eventID any
	if strings.TrimSpace(req.EventID) != "" {
		eventID = req.EventID
	}
	commandTag, err := s.db.Exec(ctx, `
		INSERT INTO timeline_events (id, event_id, resource_type, resource_id, event_type, correlation_id, source, payload, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)
		ON CONFLICT DO NOTHING
	`, id, eventID, req.ResourceType, req.ResourceID, req.EventType, req.CorrelationID, req.Source, string(payloadBytes), req.Timestamp)
	if err != nil {
		return TimelineEvent{}, err
	}
	if commandTag.RowsAffected() == 0 && req.EventID != "" {
		return s.GetTimelineEventByEventID(ctx, req.EventID)
	}
	return s.GetTimelineEvent(ctx, id)
}

func (s *Store) GetTimelineEvent(ctx context.Context, id string) (TimelineEvent, error) {
	return s.getTimelineEvent(ctx, `id = $1`, id)
}

func (s *Store) GetTimelineEventByEventID(ctx context.Context, eventID string) (TimelineEvent, error) {
	return s.getTimelineEvent(ctx, `event_id = $1`, eventID)
}

func (s *Store) getTimelineEvent(ctx context.Context, predicate string, value string) (TimelineEvent, error) {
	var event TimelineEvent
	var eventID sql.NullString
	var payloadBytes []byte
	err := s.db.QueryRow(ctx, `
		SELECT id::text, event_id::text, resource_type, resource_id, event_type, correlation_id, source, created_at, payload
		FROM timeline_events
		WHERE `+predicate+`
	`, value).Scan(&event.ID, &eventID, &event.ResourceType, &event.ResourceID, &event.EventType, &event.CorrelationID, &event.Source, &event.Timestamp, &payloadBytes)
	if err != nil {
		return TimelineEvent{}, err
	}
	if eventID.Valid {
		event.EventID = eventID.String
	}
	event.Payload = decodePayload(payloadBytes)
	return event, nil
}

func (s *Store) ListTimelineEvents(ctx context.Context, query TimelineQuery) ([]TimelineEvent, error) {
	limit := normalizeTimelineLimit(query.Limit)
	rows, err := s.db.Query(ctx, `
		SELECT id::text, event_id::text, resource_type, resource_id, event_type, correlation_id, source, created_at, payload
		FROM timeline_events
		WHERE ($1 = '' OR resource_type = $1)
		  AND ($2 = '' OR resource_id = $2)
		  AND ($3 = '' OR correlation_id = $3)
		ORDER BY created_at DESC, id DESC
		LIMIT $4
	`, strings.TrimSpace(query.ResourceType), strings.TrimSpace(query.ResourceID), strings.TrimSpace(query.CorrelationID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []TimelineEvent{}
	for rows.Next() {
		var event TimelineEvent
		var eventID sql.NullString
		var payloadBytes []byte
		if err := rows.Scan(&event.ID, &eventID, &event.ResourceType, &event.ResourceID, &event.EventType, &event.CorrelationID, &event.Source, &event.Timestamp, &payloadBytes); err != nil {
			return nil, err
		}
		if eventID.Valid {
			event.EventID = eventID.String
		}
		event.Payload = decodePayload(payloadBytes)
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) CreateNodeHeartbeatHistory(ctx context.Context, req CreateNodeHeartbeatHistoryRequest) (NodeHeartbeatHistory, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	var previous sql.NullTime
	_ = s.db.QueryRow(ctx, `SELECT observed_at FROM node_heartbeat_history WHERE node_id = $1 ORDER BY observed_at DESC LIMIT 1`, req.NodeID).Scan(&previous)
	var gapSeconds any
	if previous.Valid {
		gap := int(now.Sub(previous.Time).Seconds())
		if gap < 0 {
			gap = 0
		}
		gapSeconds = gap
	}
	if req.PreviousSeenAt != nil {
		previous = sql.NullTime{Time: *req.PreviousSeenAt, Valid: true}
		gap := int(now.Sub(*req.PreviousSeenAt).Seconds())
		if gap < 0 {
			gap = 0
		}
		gapSeconds = gap
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO node_heartbeat_history (
			id, node_id, observed_at, previous_seen_at, gap_seconds, success, failure_reason,
			version, os, architecture, cpu_threads, memory_mb, disk_mb, runtime_status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, 0), NULLIF($12, 0), NULLIF($13, 0), NULLIF($14, ''))
	`, id, req.NodeID, now, nullableTime(previous), gapSeconds, req.Success, req.FailureReason, req.Version, req.OS, req.Architecture, req.CPUThreads, req.MemoryMB, req.DiskMB, req.RuntimeStatus)
	if err != nil {
		return NodeHeartbeatHistory{}, err
	}
	history, err := s.ListNodeHeartbeatHistory(ctx, req.NodeID, 1)
	if err != nil || len(history) == 0 {
		return NodeHeartbeatHistory{}, err
	}
	return history[0], nil
}

func (s *Store) ListNodeHeartbeatHistory(ctx context.Context, nodeID string, limit int) ([]NodeHeartbeatHistory, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, node_id::text, observed_at, previous_seen_at, gap_seconds, success, failure_reason,
		       version, os, architecture, cpu_threads, memory_mb, disk_mb, runtime_status
		FROM node_heartbeat_history
		WHERE node_id = $1
		ORDER BY observed_at DESC, id DESC
		LIMIT $2
	`, nodeID, normalizeTimelineLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := []NodeHeartbeatHistory{}
	for rows.Next() {
		var item NodeHeartbeatHistory
		var previous sql.NullTime
		var gap sql.NullInt32
		var version, osName, architecture, runtimeStatus sql.NullString
		var cpuThreads, memoryMB, diskMB sql.NullInt32
		if err := rows.Scan(&item.ID, &item.NodeID, &item.ObservedAt, &previous, &gap, &item.Success, &item.FailureReason, &version, &osName, &architecture, &cpuThreads, &memoryMB, &diskMB, &runtimeStatus); err != nil {
			return nil, err
		}
		if previous.Valid {
			item.PreviousSeenAt = &previous.Time
		}
		if gap.Valid {
			value := int(gap.Int32)
			item.GapSeconds = &value
		}
		item.Version = nullString(version)
		item.OS = nullString(osName)
		item.Architecture = nullString(architecture)
		item.RuntimeStatus = nullString(runtimeStatus)
		item.CPUThreads = nullInt(cpuThreads)
		item.MemoryMB = nullInt(memoryMB)
		item.DiskMB = nullInt(diskMB)
		history = append(history, item)
	}
	return history, rows.Err()
}

func (s *Store) CreateNodeHealthHistory(ctx context.Context, req CreateNodeHealthHistoryRequest) (NodeHealthHistory, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO node_health_history (
			id, node_id, actual_state, desired_state, health_score, cpu_score, memory_score, disk_score,
			heartbeat_score, status_score, allocated_cpu, available_cpu, allocated_memory, available_memory,
			allocated_disk, available_disk, server_count
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, id, req.NodeID, req.ActualState, req.DesiredState, req.HealthScore, req.CPUScore, req.MemoryScore, req.DiskScore,
		req.HeartbeatScore, req.StatusScore, req.AllocatedCPU, req.AvailableCPU, req.AllocatedMemory, req.AvailableMemory,
		req.AllocatedDisk, req.AvailableDisk, req.ServerCount)
	if err != nil {
		return NodeHealthHistory{}, err
	}
	history, err := s.ListNodeHealthHistory(ctx, req.NodeID, 1)
	if err != nil || len(history) == 0 {
		return NodeHealthHistory{}, err
	}
	return history[0], nil
}

func (s *Store) ListNodeHealthHistory(ctx context.Context, nodeID string, limit int) ([]NodeHealthHistory, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, node_id::text, observed_at, actual_state, desired_state, health_score, cpu_score, memory_score,
		       disk_score, heartbeat_score, status_score, allocated_cpu, available_cpu, allocated_memory, available_memory,
		       allocated_disk, available_disk, server_count
		FROM node_health_history
		WHERE node_id = $1
		ORDER BY observed_at DESC, id DESC
		LIMIT $2
	`, nodeID, normalizeTimelineLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := []NodeHealthHistory{}
	for rows.Next() {
		var item NodeHealthHistory
		if err := rows.Scan(&item.ID, &item.NodeID, &item.ObservedAt, &item.ActualState, &item.DesiredState, &item.HealthScore, &item.CPUScore, &item.MemoryScore,
			&item.DiskScore, &item.HeartbeatScore, &item.StatusScore, &item.AllocatedCPU, &item.AvailableCPU, &item.AllocatedMemory, &item.AvailableMemory,
			&item.AllocatedDisk, &item.AvailableDisk, &item.ServerCount); err != nil {
			return nil, err
		}
		history = append(history, item)
	}
	return history, rows.Err()
}

func (s *Store) TimelineEventsTotal(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM timeline_events`).Scan(&total)
	return total, err
}

func (s *Store) CorrelationGroupsTotal(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(DISTINCT correlation_id) FROM timeline_events`).Scan(&total)
	return total, err
}

func (s *Store) HeartbeatFailuresTotal(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM node_heartbeat_history WHERE success = false`).Scan(&total)
	return total, err
}

func (s *Store) HealthSnapshotsTotal(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM node_health_history`).Scan(&total)
	return total, err
}

func decodePayload(payloadBytes []byte) map[string]any {
	payload := map[string]any{}
	if len(payloadBytes) == 0 {
		return payload
	}
	_ = json.Unmarshal(payloadBytes, &payload)
	return payload
}

func normalizeTimelineLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func nullableTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}

func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullInt(value sql.NullInt32) *int {
	if !value.Valid {
		return nil
	}
	out := int(value.Int32)
	return &out
}
