package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type AlertSeverity string

const (
	AlertSeverityOK       AlertSeverity = "ok"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityCritical AlertSeverity = "critical"
)

type Alert struct {
	ID             string         `json:"id"`
	NodeID         string         `json:"nodeId"`
	ServerID       string         `json:"serverId"`
	AlertType      string         `json:"alertType"`
	Severity       AlertSeverity  `json:"severity"`
	Title          string         `json:"title"`
	Message        string         `json:"message"`
	Details        map[string]any `json:"details"`
	Source         string         `json:"source"`
	Acknowledged   bool           `json:"acknowledged"`
	AcknowledgedBy string         `json:"acknowledgedBy"`
	AcknowledgedAt *time.Time     `json:"acknowledgedAt"`
	ResolvedAt     *time.Time     `json:"resolvedAt"`
	SuppressionKey string         `json:"suppressionKey"`
	TenantID       string         `json:"tenantId"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}

type CreateAlertRequest struct {
	NodeID         string
	ServerID       string
	AlertType      string
	Severity       AlertSeverity
	Title          string
	Message        string
	Details        map[string]any
	Source         string
	SuppressionKey string
	TenantID       string
}

type AlertFilter struct {
	NodeID         *string
	ServerID       *string
	Severity       *AlertSeverity
	Acknowledged   *bool
	AlertType      *string
	Source         *string
	SuppressionKey *string
	TenantID       *string
	From           *time.Time
	To             *time.Time
	Limit          int
	Offset         int
}

type NotificationRoute struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	ChannelType string         `json:"channelType"`
	Enabled     bool           `json:"enabled"`
	Config      map[string]any `json:"config"`
	MinSeverity AlertSeverity  `json:"minSeverity"`
	EventTypes  []string       `json:"eventTypes"`
	TenantID    string         `json:"tenantId"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type CreateNotificationRouteRequest struct {
	Name        string
	ChannelType string
	Enabled     bool
	Config      map[string]any
	MinSeverity AlertSeverity
	EventTypes  []string
	TenantID    string
}

type HealthHistoryRecord struct {
	ID         string         `json:"id"`
	CheckName  string         `json:"checkName"`
	Status     string         `json:"status"`
	Message    string         `json:"message"`
	LatencyMs  int64          `json:"latencyMs"`
	Details    map[string]any `json:"details"`
	Critical   bool           `json:"critical"`
	ObservedAt time.Time      `json:"observedAt"`
}

type RetentionPolicy struct {
	ID         string    `json:"id"`
	MetricType string    `json:"metricType"`
	TTLHours   int       `json:"ttlHours"`
	MaxRecords int       `json:"maxRecords"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (s *Store) CreateAlert(ctx context.Context, req CreateAlertRequest) (Alert, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	if req.Source == "" {
		req.Source = "system"
	}
	if req.Severity == "" {
		req.Severity = AlertSeverityWarning
	}
	details := req.Details
	if details == nil {
		details = map[string]any{}
	}
	detailsBytes, _ := json.Marshal(details)

	_, err := s.db.Exec(ctx, `
		INSERT INTO alerts (id, node_id, server_id, alert_type, severity, title, message, details, source,
			acknowledged, suppression_key, tenant_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,false,$10,$11,$12,$13)
	`, id, nullableUUID(req.NodeID), nullableUUID(req.ServerID), req.AlertType, string(req.Severity),
		req.Title, req.Message, string(detailsBytes), req.Source,
		req.SuppressionKey, req.TenantID, now, now)
	if err != nil {
		return Alert{}, err
	}
	return s.GetAlert(ctx, id)
}

func (s *Store) GetAlert(ctx context.Context, id string) (Alert, error) {
	var a Alert
	var nodeID, serverID, acknowledgedBy, suppressionKey, tenantID sql.NullString
	var detailsBytes []byte
	var acknowledgedAt, resolvedAt sql.NullTime
	err := s.db.QueryRow(ctx, `
		SELECT id::text, COALESCE(node_id::text,''), COALESCE(server_id::text,''),
			alert_type, severity, title, message, details,
			source, acknowledged, COALESCE(acknowledged_by,''),
			acknowledged_at, resolved_at, COALESCE(suppression_key,''), COALESCE(tenant_id,''),
			created_at, updated_at
		FROM alerts WHERE id = $1
	`, id).Scan(&a.ID, &nodeID, &serverID, &a.AlertType, &a.Severity,
		&a.Title, &a.Message, &detailsBytes,
		&a.Source, &a.Acknowledged, &acknowledgedBy,
		&acknowledgedAt, &resolvedAt, &suppressionKey, &tenantID,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return Alert{}, err
	}
	a.NodeID = nodeID.String
	a.ServerID = serverID.String
	a.AcknowledgedBy = acknowledgedBy.String
	a.SuppressionKey = suppressionKey.String
	a.TenantID = tenantID.String
	if acknowledgedAt.Valid {
		a.AcknowledgedAt = &acknowledgedAt.Time
	}
	if resolvedAt.Valid {
		a.ResolvedAt = &resolvedAt.Time
	}
	if len(detailsBytes) > 0 {
		json.Unmarshal(detailsBytes, &a.Details)
	}
	if a.Details == nil {
		a.Details = map[string]any{}
	}
	return a, nil
}

func (s *Store) ListAlerts(ctx context.Context, filter AlertFilter) ([]Alert, error) {
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 100
	}
	query := `SELECT id::text, COALESCE(node_id::text,''), COALESCE(server_id::text,''),
		alert_type, severity, title, message, details,
		source, acknowledged, COALESCE(acknowledged_by,''),
		acknowledged_at, resolved_at, COALESCE(suppression_key,''), COALESCE(tenant_id,''),
		created_at, updated_at FROM alerts WHERE 1=1`
	args := []any{}
	argN := 1

	if filter.NodeID != nil {
		query += ` AND node_id = $` + itoa(argN)
		args = append(args, *filter.NodeID)
		argN++
	}
	if filter.ServerID != nil {
		query += ` AND server_id = $` + itoa(argN)
		args = append(args, *filter.ServerID)
		argN++
	}
	if filter.Severity != nil {
		query += ` AND severity = $` + itoa(argN)
		args = append(args, string(*filter.Severity))
		argN++
	}
	if filter.Acknowledged != nil {
		query += ` AND acknowledged = $` + itoa(argN)
		args = append(args, *filter.Acknowledged)
		argN++
	}
	if filter.AlertType != nil {
		query += ` AND alert_type = $` + itoa(argN)
		args = append(args, *filter.AlertType)
		argN++
	}
	if filter.Source != nil {
		query += ` AND source = $` + itoa(argN)
		args = append(args, *filter.Source)
		argN++
	}
	if filter.SuppressionKey != nil {
		query += ` AND suppression_key = $` + itoa(argN)
		args = append(args, *filter.SuppressionKey)
		argN++
	}
	if filter.TenantID != nil {
		query += ` AND tenant_id = $` + itoa(argN)
		args = append(args, *filter.TenantID)
		argN++
	}
	if filter.From != nil {
		query += ` AND created_at >= $` + itoa(argN)
		args = append(args, *filter.From)
		argN++
	}
	if filter.To != nil {
		query += ` AND created_at <= $` + itoa(argN)
		args = append(args, *filter.To)
		argN++
	}
	query += ` ORDER BY created_at DESC`
	query += ` LIMIT $` + itoa(argN)
	args = append(args, filter.Limit)
	argN++
	if filter.Offset > 0 {
		query += ` OFFSET $` + itoa(argN)
		args = append(args, filter.Offset)
		argN++
	}

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Alert{}
	for rows.Next() {
		var a Alert
		var nodeID, serverID, acknowledgedBy, suppressionKey, tenantID sql.NullString
		var detailsBytes []byte
		var acknowledgedAt, resolvedAt sql.NullTime
		if err := rows.Scan(&a.ID, &nodeID, &serverID,
			&a.AlertType, &a.Severity, &a.Title, &a.Message, &detailsBytes,
			&a.Source, &a.Acknowledged, &acknowledgedBy,
			&acknowledgedAt, &resolvedAt, &suppressionKey, &tenantID,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.NodeID = nodeID.String
		a.ServerID = serverID.String
		a.AcknowledgedBy = acknowledgedBy.String
		a.SuppressionKey = suppressionKey.String
		a.TenantID = tenantID.String
		if acknowledgedAt.Valid {
			a.AcknowledgedAt = &acknowledgedAt.Time
		}
		if resolvedAt.Valid {
			a.ResolvedAt = &resolvedAt.Time
		}
		if len(detailsBytes) > 0 {
			json.Unmarshal(detailsBytes, &a.Details)
		}
		if a.Details == nil {
			a.Details = map[string]any{}
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (s *Store) CountAlerts(ctx context.Context, filter AlertFilter) (int, error) {
	query := `SELECT COUNT(*) FROM alerts WHERE 1=1`
	args := []any{}
	argN := 1

	if filter.NodeID != nil {
		query += ` AND node_id = $` + itoa(argN)
		args = append(args, *filter.NodeID)
		argN++
	}
	if filter.Severity != nil {
		query += ` AND severity = $` + itoa(argN)
		args = append(args, string(*filter.Severity))
		argN++
	}
	if filter.Acknowledged != nil {
		query += ` AND acknowledged = $` + itoa(argN)
		args = append(args, *filter.Acknowledged)
		argN++
	}
	if filter.TenantID != nil {
		query += ` AND tenant_id = $` + itoa(argN)
		args = append(args, *filter.TenantID)
		argN++
	}

	var count int
	err := s.db.QueryRow(ctx, query, args...).Scan(&count)
	return count, err
}

func (s *Store) AcknowledgeAlert(ctx context.Context, id, acknowledgedBy string) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		UPDATE alerts SET acknowledged = true, acknowledged_by = $1, acknowledged_at = $2, updated_at = $2
		WHERE id = $3
	`, acknowledgedBy, now, id)
	return err
}

func (s *Store) ResolveAlert(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		UPDATE alerts SET severity = 'ok', resolved_at = $1, updated_at = $1 WHERE id = $2
	`, now, id)
	return err
}

func (s *Store) FindAlertBySuppressionKey(ctx context.Context, suppressionKey string) (*Alert, error) {
	rows, err := s.ListAlerts(ctx, AlertFilter{SuppressionKey: &suppressionKey, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (s *Store) CreateNotificationRoute(ctx context.Context, req CreateNotificationRouteRequest) (NotificationRoute, error) {
	id := uuid.NewString()
	config := req.Config
	if config == nil {
		config = map[string]any{}
	}
	configBytes, _ := json.Marshal(config)
	eventTypes := req.EventTypes
	if eventTypes == nil {
		eventTypes = []string{}
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO notification_routes (id, name, channel_type, enabled, config, min_severity, event_types, tenant_id)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8)
	`, id, req.Name, req.ChannelType, req.Enabled, string(configBytes),
		string(req.MinSeverity), eventTypes, req.TenantID)
	if err != nil {
		return NotificationRoute{}, err
	}
	return s.GetNotificationRoute(ctx, id)
}

func (s *Store) GetNotificationRoute(ctx context.Context, id string) (NotificationRoute, error) {
	var r NotificationRoute
	var configBytes []byte
	var eventTypes []string
	var tenantID sql.NullString
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, channel_type, enabled, config, min_severity, event_types, COALESCE(tenant_id,''), created_at, updated_at
		FROM notification_routes WHERE id = $1
	`, id).Scan(&r.ID, &r.Name, &r.ChannelType, &r.Enabled, &configBytes,
		&r.MinSeverity, &eventTypes, &tenantID, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return NotificationRoute{}, err
	}
	r.TenantID = tenantID.String
	r.EventTypes = eventTypes
	if len(configBytes) > 0 {
		json.Unmarshal(configBytes, &r.Config)
	}
	if r.Config == nil {
		r.Config = map[string]any{}
	}
	return r, nil
}

func (s *Store) ListNotificationRoutes(ctx context.Context, tenantID string) ([]NotificationRoute, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, channel_type, enabled, config, min_severity, event_types, COALESCE(tenant_id,''), created_at, updated_at
		FROM notification_routes
		WHERE ($1 = '' OR tenant_id = $1)
		ORDER BY name ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []NotificationRoute{}
	for rows.Next() {
		var r NotificationRoute
		var configBytes []byte
		var eventTypes []string
		var tenantID sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &r.ChannelType, &r.Enabled, &configBytes,
			&r.MinSeverity, &eventTypes, &tenantID, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.TenantID = tenantID.String
		r.EventTypes = eventTypes
		if len(configBytes) > 0 {
			json.Unmarshal(configBytes, &r.Config)
		}
		if r.Config == nil {
			r.Config = map[string]any{}
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) DeleteNotificationRoute(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM notification_routes WHERE id = $1`, id)
	return err
}

func (s *Store) CreateHealthHistory(ctx context.Context, rec HealthHistoryRecord) error {
	id := uuid.NewString()
	if rec.ObservedAt.IsZero() {
		rec.ObservedAt = time.Now().UTC()
	}
	details := rec.Details
	if details == nil {
		details = map[string]any{}
	}
	detailsBytes, _ := json.Marshal(details)
	_, err := s.db.Exec(ctx, `
		INSERT INTO health_history (id, check_name, status, message, latency_ms, details, critical, observed_at)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8)
	`, id, rec.CheckName, rec.Status, rec.Message, rec.LatencyMs, string(detailsBytes), rec.Critical, rec.ObservedAt)
	return err
}

func (s *Store) ListHealthHistory(ctx context.Context, checkName string, limit int) ([]HealthHistoryRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, check_name, status, COALESCE(message,''), COALESCE(latency_ms,0), details, critical, observed_at
		FROM health_history
		WHERE check_name = $1
		ORDER BY observed_at DESC LIMIT $2
	`, checkName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []HealthHistoryRecord{}
	for rows.Next() {
		var rec HealthHistoryRecord
		var detailsBytes []byte
		if err := rows.Scan(&rec.ID, &rec.CheckName, &rec.Status, &rec.Message, &rec.LatencyMs, &detailsBytes, &rec.Critical, &rec.ObservedAt); err != nil {
			return nil, err
		}
		if len(detailsBytes) > 0 {
			json.Unmarshal(detailsBytes, &rec.Details)
		}
		if rec.Details == nil {
			rec.Details = map[string]any{}
		}
		result = append(result, rec)
	}
	return result, rows.Err()
}

func (s *Store) ListAllHealthHistory(ctx context.Context, limit int) ([]HealthHistoryRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, check_name, status, COALESCE(message,''), COALESCE(latency_ms,0), details, critical, observed_at
		FROM health_history
		ORDER BY observed_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []HealthHistoryRecord{}
	for rows.Next() {
		var rec HealthHistoryRecord
		var detailsBytes []byte
		if err := rows.Scan(&rec.ID, &rec.CheckName, &rec.Status, &rec.Message, &rec.LatencyMs, &detailsBytes, &rec.Critical, &rec.ObservedAt); err != nil {
			return nil, err
		}
		if len(detailsBytes) > 0 {
			json.Unmarshal(detailsBytes, &rec.Details)
		}
		if rec.Details == nil {
			rec.Details = map[string]any{}
		}
		result = append(result, rec)
	}
	return result, rows.Err()
}

func (s *Store) GetRetentionPolicy(ctx context.Context, metricType string) (*RetentionPolicy, error) {
	var p RetentionPolicy
	err := s.db.QueryRow(ctx, `
		SELECT id::text, metric_type, ttl_hours, max_records, enabled, created_at, updated_at
		FROM retention_policies WHERE metric_type = $1
	`, metricType).Scan(&p.ID, &p.MetricType, &p.TTLHours, &p.MaxRecords, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListRetentionPolicies(ctx context.Context) ([]RetentionPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, metric_type, ttl_hours, max_records, enabled, created_at, updated_at
		FROM retention_policies ORDER BY metric_type
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RetentionPolicy{}
	for rows.Next() {
		var p RetentionPolicy
		if err := rows.Scan(&p.ID, &p.MetricType, &p.TTLHours, &p.MaxRecords, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *Store) UpdateRetentionPolicy(ctx context.Context, metricType string, ttlHours int, maxRecords int) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO retention_policies (id, metric_type, ttl_hours, max_records)
		VALUES (gen_random_uuid()::text, $1, $2, $3)
		ON CONFLICT (metric_type) DO UPDATE SET
			ttl_hours = EXCLUDED.ttl_hours,
			max_records = EXCLUDED.max_records,
			updated_at = now()
	`, metricType, ttlHours, maxRecords)
	return err
}

func (s *Store) EnforceRetention(ctx context.Context) (map[string]int64, error) {
	policies, err := s.ListRetentionPolicies(ctx)
	if err != nil {
		return nil, err
	}
	deleted := map[string]int64{}
	for _, p := range policies {
		if !p.Enabled || p.TTLHours <= 0 {
			continue
		}
		before := time.Now().UTC().Add(-time.Duration(p.TTLHours) * time.Hour)
		var count int64
		switch p.MetricType {
		case "node_metrics":
			count, err = s.PruneNodeMetrics(ctx, before)
		case "workload_metrics":
			count, err = s.PruneWorkloadMetrics(ctx, before)
		case "build_logs":
			count, err = s.PruneBuildLogs(ctx, before)
		case "deployment_logs":
			count, err = s.PruneDeploymentLogs(ctx, before)
		case "beacon_command_logs":
			count, err = s.PruneBeaconCommandLogs(ctx, before)
		case "alerts":
			count, err = s.PruneAlerts(ctx, before)
		case "health_history":
			count, err = s.PruneHealthHistory(ctx, before)
		}
		if err == nil && count > 0 {
			deleted[p.MetricType] = count
		}
	}
	return deleted, nil
}

func (s *Store) PruneAlerts(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.Exec(ctx, `DELETE FROM alerts WHERE created_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

func (s *Store) PruneBuildLogs(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.Exec(ctx, `DELETE FROM build_logs WHERE created_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

func (s *Store) PruneDeploymentLogs(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.Exec(ctx, `DELETE FROM deployment_logs WHERE created_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

func (s *Store) PruneBeaconCommandLogs(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.Exec(ctx, `DELETE FROM beacon_command_logs WHERE created_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

func (s *Store) PruneHealthHistory(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.Exec(ctx, `DELETE FROM health_history WHERE observed_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}
