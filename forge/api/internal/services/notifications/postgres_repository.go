package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements Repository interface using PostgreSQL
type PostgresRepository struct {
	db *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgresRepository
func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// Notification Channel operations

func (r *PostgresRepository) CreateNotificationChannel(ctx context.Context, req CreateNotificationChannelRequest) (NotificationChannel, error) {
	id := uuid.NewString()
	configBytes, _ := json.Marshal(req.Config)
	now := time.Now().UTC()

	_, err := r.db.Exec(ctx, `
		INSERT INTO notification_channels (id, tenant_id, user_id, type, name, config, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9)
	`, id, req.TenantID, req.UserID, string(req.Type), req.Name, string(configBytes), req.IsActive, now, now)
	if err != nil {
		return NotificationChannel{}, fmt.Errorf("failed to create notification channel: %w", err)
	}

	return r.GetNotificationChannel(ctx, id)
}

func (r *PostgresRepository) GetNotificationChannel(ctx context.Context, id string) (NotificationChannel, error) {
	var ch NotificationChannel
	var configBytes []byte
	var userID *string

	err := r.db.QueryRow(ctx, `
		SELECT id::text, tenant_id, user_id, type, name, config, is_active, created_at, updated_at
		FROM notification_channels WHERE id = $1
	`, id).Scan(&ch.ID, &ch.TenantID, &userID, &ch.Type, &ch.Name, &configBytes, &ch.IsActive, &ch.CreatedAt, &ch.UpdatedAt)
	if err != nil {
		return NotificationChannel{}, fmt.Errorf("failed to get notification channel: %w", err)
	}

	ch.UserID = userID
	if len(configBytes) > 0 {
		json.Unmarshal(configBytes, &ch.Config)
	}

	return ch, nil
}

func (r *PostgresRepository) ListNotificationChannels(ctx context.Context, tenantID *string, userID *string) ([]NotificationChannel, error) {
	query := `
		SELECT id::text, tenant_id, user_id, type, name, config, is_active, created_at, updated_at
		FROM notification_channels
	`
	var args []interface{}
	var conditions []string

	if tenantID != nil && *tenantID != "" {
		conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", len(args)+1))
		args = append(args, *tenantID)
	}

	if userID != nil && *userID != "" {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", len(args)+1))
		args = append(args, *userID)
	}

	if len(conditions) > 0 {
		query += " WHERE " + joinConditions(conditions)
	}

	query += " ORDER BY name ASC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification channels: %w", err)
	}
	defer rows.Close()

	result := []NotificationChannel{}
	for rows.Next() {
		var ch NotificationChannel
		var configBytes []byte
		var userID *string

		if err := rows.Scan(&ch.ID, &ch.TenantID, &userID, &ch.Type, &ch.Name, &configBytes, &ch.IsActive, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan notification channel: %w", err)
		}

		ch.UserID = userID
		if len(configBytes) > 0 {
			json.Unmarshal(configBytes, &ch.Config)
		}

		result = append(result, ch)
	}

	return result, rows.Err()
}

func (r *PostgresRepository) UpdateNotificationChannel(ctx context.Context, id string, req UpdateNotificationChannelRequest) (NotificationChannel, error) {
	existing, err := r.GetNotificationChannel(ctx, id)
	if err != nil {
		return NotificationChannel{}, err
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Config != nil {
		existing.Config = *req.Config
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	configBytes, _ := json.Marshal(existing.Config)
	now := time.Now().UTC()

	_, err = r.db.Exec(ctx, `
		UPDATE notification_channels SET name=$1, config=$2::jsonb, is_active=$3, updated_at=$4 WHERE id=$5
	`, existing.Name, string(configBytes), existing.IsActive, now, id)
	if err != nil {
		return NotificationChannel{}, fmt.Errorf("failed to update notification channel: %w", err)
	}

	return r.GetNotificationChannel(ctx, id)
}

func (r *PostgresRepository) DeleteNotificationChannel(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM notification_channels WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete notification channel: %w", err)
	}
	return nil
}

// Alert Rule operations

func (r *PostgresRepository) CreateAlertRule(ctx context.Context, req CreateAlertRuleRequest) (AlertRule, error) {
	id := uuid.NewString()
	now := time.Now().UTC()

	_, err := r.db.Exec(ctx, `
		INSERT INTO alert_rules (
			id, tenant_id, user_id, name, description, rule_type, entity_type,
			metric_name, threshold_value, comparison_operator, state_value,
			duration_minutes, cooldown_minutes, severity, is_enabled,
			notification_channel_ids, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16::uuid[], $17, $18)
	`,
		id, req.TenantID, req.UserID, req.Name, req.Description, string(req.RuleType), string(req.EntityType),
		req.MetricName, req.ThresholdValue, req.ComparisonOperator, req.StateValue,
		req.DurationMinutes, req.CooldownMinutes, string(req.Severity), req.IsEnabled,
		req.NotificationChannelIDs, now, now)
	if err != nil {
		return AlertRule{}, fmt.Errorf("failed to create alert rule: %w", err)
	}

	return r.GetAlertRule(ctx, id)
}

func (r *PostgresRepository) GetAlertRule(ctx context.Context, id string) (AlertRule, error) {
	var rule AlertRule
	var description, metricName, stateValue *string
	var thresholdValue, durationMinutes, cooldownMinutes *int
	var comparisonOperator *string

	err := r.db.QueryRow(ctx, `
		SELECT id::text, tenant_id, user_id, name, description, rule_type, entity_type,
		metric_name, threshold_value, comparison_operator, state_value,
		duration_minutes, cooldown_minutes, severity, is_enabled, notification_channel_ids,
		created_at, updated_at, last_triggered_at
		FROM alert_rules WHERE id = $1
	`, id).Scan(
		&rule.ID, &rule.TenantID, &rule.UserID, &rule.Name, &description, &rule.RuleType, &rule.EntityType,
		&metricName, &thresholdValue, &comparisonOperator, &stateValue,
		&durationMinutes, &cooldownMinutes, &rule.Severity, &rule.IsEnabled, &rule.NotificationChannelIDs,
		&rule.CreatedAt, &rule.UpdatedAt, &rule.LastTriggeredAt)
	if err != nil {
		return AlertRule{}, fmt.Errorf("failed to get alert rule: %w", err)
	}

	rule.Description = description
	rule.MetricName = metricName
	rule.StateValue = stateValue

	if thresholdValue != nil {
		floatVal := float64(*thresholdValue)
		rule.ThresholdValue = &floatVal
	}
	if comparisonOperator != nil {
		op := AlertComparisonOperator(*comparisonOperator)
		rule.ComparisonOperator = &op
	}
	if durationMinutes != nil {
		rule.DurationMinutes = *durationMinutes
	}
	if cooldownMinutes != nil {
		rule.CooldownMinutes = *cooldownMinutes
	}

	return rule, nil
}

func (r *PostgresRepository) ListAlertRules(ctx context.Context, tenantID *string, userID *string, entityType *EntityType) ([]AlertRule, error) {
	query := `
		SELECT id::text, tenant_id, user_id, name, description, rule_type, entity_type,
		metric_name, threshold_value, comparison_operator, state_value,
		duration_minutes, cooldown_minutes, severity, is_enabled, notification_channel_ids,
		created_at, updated_at, last_triggered_at
		FROM alert_rules
	`
	var args []interface{}
	var conditions []string

	if tenantID != nil && *tenantID != "" {
		conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", len(args)+1))
		args = append(args, *tenantID)
	}

	if userID != nil && *userID != "" {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", len(args)+1))
		args = append(args, *userID)
	}

	if entityType != nil && *entityType != "" {
		conditions = append(conditions, fmt.Sprintf("entity_type = $%d", len(args)+1))
		args = append(args, string(*entityType))
	}

	if len(conditions) > 0 {
		query += " WHERE " + joinConditions(conditions)
	}

	query += " ORDER BY name ASC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list alert rules: %w", err)
	}
	defer rows.Close()

	result := []AlertRule{}
	for rows.Next() {
		var rule AlertRule
		var description, metricName, stateValue *string
		var thresholdValue, durationMinutes, cooldownMinutes *int
		var comparisonOperator *string

		if err := rows.Scan(
			&rule.ID, &rule.TenantID, &rule.UserID, &rule.Name, &description, &rule.RuleType, &rule.EntityType,
			&metricName, &thresholdValue, &comparisonOperator, &stateValue,
			&durationMinutes, &cooldownMinutes, &rule.Severity, &rule.IsEnabled, &rule.NotificationChannelIDs,
			&rule.CreatedAt, &rule.UpdatedAt, &rule.LastTriggeredAt); err != nil {
			return nil, fmt.Errorf("failed to scan alert rule: %w", err)
		}

		rule.Description = description
		rule.MetricName = metricName
		rule.StateValue = stateValue

		if thresholdValue != nil {
			floatVal := float64(*thresholdValue)
			rule.ThresholdValue = &floatVal
		}
		if comparisonOperator != nil {
			op := AlertComparisonOperator(*comparisonOperator)
			rule.ComparisonOperator = &op
		}
		if durationMinutes != nil {
			rule.DurationMinutes = *durationMinutes
		}
		if cooldownMinutes != nil {
			rule.CooldownMinutes = *cooldownMinutes
		}

		result = append(result, rule)
	}

	return result, rows.Err()
}

func (r *PostgresRepository) UpdateAlertRule(ctx context.Context, id string, req UpdateAlertRuleRequest) (AlertRule, error) {
	existing, err := r.GetAlertRule(ctx, id)
	if err != nil {
		return AlertRule{}, err
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = req.Description
	}
	if req.RuleType != nil {
		existing.RuleType = *req.RuleType
	}
	if req.EntityType != nil {
		existing.EntityType = *req.EntityType
	}
	if req.MetricName != nil {
		existing.MetricName = req.MetricName
	}
	if req.ThresholdValue != nil {
		existing.ThresholdValue = req.ThresholdValue
	}
	if req.ComparisonOperator != nil {
		existing.ComparisonOperator = req.ComparisonOperator
	}
	if req.StateValue != nil {
		existing.StateValue = req.StateValue
	}
	if req.DurationMinutes != nil {
		existing.DurationMinutes = *req.DurationMinutes
	}
	if req.CooldownMinutes != nil {
		existing.CooldownMinutes = *req.CooldownMinutes
	}
	if req.Severity != nil {
		existing.Severity = *req.Severity
	}
	if req.IsEnabled != nil {
		existing.IsEnabled = *req.IsEnabled
	}
	if req.NotificationChannelIDs != nil {
		existing.NotificationChannelIDs = *req.NotificationChannelIDs
	}

	now := time.Now().UTC()

	_, err = r.db.Exec(ctx, `
		UPDATE alert_rules SET
			name = $1, description = $2, rule_type = $3, entity_type = $4,
			metric_name = $5, threshold_value = $6, comparison_operator = $7, state_value = $8,
			duration_minutes = $9, cooldown_minutes = $10, severity = $11, is_enabled = $12,
			notification_channel_ids = $13::uuid[], updated_at = $14
		WHERE id = $15
	`,
		existing.Name, existing.Description, string(existing.RuleType), string(existing.EntityType),
		existing.MetricName, existing.ThresholdValue, existing.ComparisonOperator, existing.StateValue,
		existing.DurationMinutes, existing.CooldownMinutes, string(existing.Severity), existing.IsEnabled,
		existing.NotificationChannelIDs, now, id)
	if err != nil {
		return AlertRule{}, fmt.Errorf("failed to update alert rule: %w", err)
	}

	return r.GetAlertRule(ctx, id)
}

func (r *PostgresRepository) DeleteAlertRule(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM alert_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete alert rule: %w", err)
	}
	return nil
}

// Alert State operations

func (r *PostgresRepository) CreateAlertState(ctx context.Context, ruleID, entityID string, entityType EntityType, currentValue *float64, currentState *string) (AlertState, error) {
	id := uuid.NewString()
	now := time.Now().UTC()

	_, err := r.db.Exec(ctx, `
		INSERT INTO alert_states (
			id, alert_rule_id, entity_id, entity_type, current_value, current_state,
			triggered_at, is_active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (alert_rule_id, entity_id, entity_type) DO UPDATE SET
			current_value = EXCLUDED.current_value,
			current_state = EXCLUDED.current_state,
			triggered_at = EXCLUDED.triggered_at,
			is_active = EXCLUDED.is_active,
			updated_at = EXCLUDED.updated_at
	`, id, ruleID, entityID, string(entityType), currentValue, currentState, nil, false, now, now)
	if err != nil {
		return AlertState{}, fmt.Errorf("failed to create alert state: %w", err)
	}

	return r.GetAlertState(ctx, ruleID, entityID, entityType)
}

func (r *PostgresRepository) GetAlertState(ctx context.Context, ruleID, entityID string, entityType EntityType) (AlertState, error) {
	var state AlertState
	var currentValue *float64
	var currentState *string
	var triggeredAt, resolvedAt *time.Time

	err := r.db.QueryRow(ctx, `
		SELECT id::text, alert_rule_id, entity_id, entity_type, current_value, current_state,
		triggered_at, resolved_at, is_active, created_at, updated_at
		FROM alert_states WHERE alert_rule_id = $1 AND entity_id = $2 AND entity_type = $3
	`, ruleID, entityID, string(entityType)).Scan(
		&state.ID, &state.AlertRuleID, &state.EntityID, &state.EntityType, &currentValue, &currentState,
		&triggeredAt, &resolvedAt, &state.IsActive, &state.CreatedAt, &state.UpdatedAt)
	if err != nil {
		return AlertState{}, fmt.Errorf("failed to get alert state: %w", err)
	}

	state.CurrentValue = currentValue
	state.CurrentState = currentState
	state.TriggeredAt = triggeredAt
	state.ResolvedAt = resolvedAt

	return state, nil
}

func (r *PostgresRepository) UpdateAlertState(ctx context.Context, id string, currentValue *float64, currentState *string, isActive bool) (AlertState, error) {
	now := time.Now().UTC()
	var triggeredAt *time.Time
	if isActive {
		t := now
		triggeredAt = &t
	}

	_, err := r.db.Exec(ctx, `
		UPDATE alert_states SET
			current_value = $1, current_state = $2, triggered_at = $3, is_active = $4, updated_at = $5
		WHERE id = $6
	`, currentValue, currentState, triggeredAt, isActive, now, id)
	if err != nil {
		return AlertState{}, fmt.Errorf("failed to update alert state: %w", err)
	}

	return r.GetAlertStateByID(ctx, id)
}

func (r *PostgresRepository) GetAlertStateByID(ctx context.Context, id string) (AlertState, error) {
	var state AlertState
	var currentValue *float64
	var currentState *string
	var triggeredAt, resolvedAt *time.Time

	err := r.db.QueryRow(ctx, `
		SELECT id::text, alert_rule_id, entity_id, entity_type, current_value, current_state,
		triggered_at, resolved_at, is_active, created_at, updated_at
		FROM alert_states WHERE id = $1
	`, id).Scan(
		&state.ID, &state.AlertRuleID, &state.EntityID, &state.EntityType, &currentValue, &currentState,
		&triggeredAt, &resolvedAt, &state.IsActive, &state.CreatedAt, &state.UpdatedAt)
	if err != nil {
		return AlertState{}, fmt.Errorf("failed to get alert state by ID: %w", err)
	}

	state.CurrentValue = currentValue
	state.CurrentState = currentState
	state.TriggeredAt = triggeredAt
	state.ResolvedAt = resolvedAt

	return state, nil
}

func (r *PostgresRepository) ListActiveAlertStates(ctx context.Context, tenantID *string) ([]AlertState, error) {
	query := `
		SELECT as.id::text, as.alert_rule_id, as.entity_id, as.entity_type, as.current_value, as.current_state,
		as.triggered_at, as.resolved_at, as.is_active, as.created_at, as.updated_at
		FROM alert_states as
		JOIN alert_rules ar ON as.alert_rule_id = ar.id
	`
	var args []interface{}
	var conditions []string

	if tenantID != nil && *tenantID != "" {
		conditions = append(conditions, fmt.Sprintf("ar.tenant_id = $%d", len(args)+1))
		args = append(args, *tenantID)
	}

	conditions = append(conditions, "as.is_active = true")

	if len(conditions) > 0 {
		query += " WHERE " + joinConditions(conditions)
	}

	query += " ORDER BY as.triggered_at DESC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list active alert states: %w", err)
	}
	defer rows.Close()

	result := []AlertState{}
	for rows.Next() {
		var state AlertState
		var currentValue *float64
		var currentState *string
		var triggeredAt, resolvedAt *time.Time

		if err := rows.Scan(
			&state.ID, &state.AlertRuleID, &state.EntityID, &state.EntityType, &currentValue, &currentState,
			&triggeredAt, &resolvedAt, &state.IsActive, &state.CreatedAt, &state.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan alert state: %w", err)
		}

		state.CurrentValue = currentValue
		state.CurrentState = currentState
		state.TriggeredAt = triggeredAt
		state.ResolvedAt = resolvedAt

		result = append(result, state)
	}

	return result, rows.Err()
}

// Notification Log operations

func (r *PostgresRepository) CreateNotificationLog(ctx context.Context, log NotificationLog) (NotificationLog, error) {
	id := uuid.NewString()
	now := time.Now().UTC()

	payloadBytes, _ := json.Marshal(log.Payload)

	_, err := r.db.Exec(ctx, `
		INSERT INTO notification_logs (
			id, tenant_id, channel_id, alert_rule_id, event_type, status, error_message, payload, sent_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10)
	`, id, log.TenantID, log.ChannelID, log.AlertRuleID, log.EventType, string(log.Status),
		log.ErrorMessage, string(payloadBytes), log.SentAt, now)
	if err != nil {
		return NotificationLog{}, fmt.Errorf("failed to create notification log: %w", err)
	}

	return r.GetNotificationLog(ctx, id)
}

func (r *PostgresRepository) GetNotificationLog(ctx context.Context, id string) (NotificationLog, error) {
	var log NotificationLog
	var alertRuleID *string
	var errorMessage *string
	var payloadBytes []byte

	err := r.db.QueryRow(ctx, `
		SELECT id::text, tenant_id, channel_id, alert_rule_id, event_type, status, error_message, payload, sent_at, created_at
		FROM notification_logs WHERE id = $1
	`, id).Scan(
		&log.ID, &log.TenantID, &log.ChannelID, &alertRuleID, &log.EventType, &log.Status,
		&errorMessage, &payloadBytes, &log.SentAt, &log.CreatedAt)
	if err != nil {
		return NotificationLog{}, fmt.Errorf("failed to get notification log: %w", err)
	}

	log.AlertRuleID = alertRuleID
	log.ErrorMessage = errorMessage
	if len(payloadBytes) > 0 {
		json.Unmarshal(payloadBytes, &log.Payload)
	}
	if log.Payload == nil {
		log.Payload = map[string]any{}
	}

	return log, nil
}

func (r *PostgresRepository) ListNotificationLogs(ctx context.Context, tenantID *string, channelID *string, limit, offset int) ([]NotificationLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := `
		SELECT id::text, tenant_id, channel_id, alert_rule_id, event_type, status, error_message, payload, sent_at, created_at
		FROM notification_logs
	`
	var args []interface{}
	var conditions []string

	if tenantID != nil && *tenantID != "" {
		conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", len(args)+1))
		args = append(args, *tenantID)
	}

	if channelID != nil && *channelID != "" {
		conditions = append(conditions, fmt.Sprintf("channel_id = $%d", len(args)+1))
		args = append(args, *channelID)
	}

	if len(conditions) > 0 {
		query += " WHERE " + joinConditions(conditions)
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification logs: %w", err)
	}
	defer rows.Close()

	result := []NotificationLog{}
	for rows.Next() {
		var log NotificationLog
		var alertRuleID *string
		var errorMessage *string
		var payloadBytes []byte

		if err := rows.Scan(
			&log.ID, &log.TenantID, &log.ChannelID, &alertRuleID, &log.EventType, &log.Status,
			&errorMessage, &payloadBytes, &log.SentAt, &log.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan notification log: %w", err)
		}

		log.AlertRuleID = alertRuleID
		log.ErrorMessage = errorMessage
		if len(payloadBytes) > 0 {
			json.Unmarshal(payloadBytes, &log.Payload)
		}
		if log.Payload == nil {
			log.Payload = map[string]any{}
		}

		result = append(result, log)
	}

	return result, rows.Err()
}

// Notification Preference operations

func (r *PostgresRepository) CreateNotificationPreference(ctx context.Context, req CreateNotificationPreferenceRequest) (NotificationPreference, error) {
	id := uuid.NewString()
	now := time.Now().UTC()

	_, err := r.db.Exec(ctx, `
		INSERT INTO notification_preferences (
			id, user_id, tenant_id, channel_type, event_types, is_enabled, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5::text[], $6, $7, $8)
		ON CONFLICT (user_id, channel_type, tenant_id) DO UPDATE SET
			event_types = EXCLUDED.event_types,
			is_enabled = EXCLUDED.is_enabled,
			updated_at = EXCLUDED.updated_at
	`, id, req.UserID, req.TenantID, req.ChannelType, req.EventTypes, req.IsEnabled, now, now)
	if err != nil {
		return NotificationPreference{}, fmt.Errorf("failed to create notification preference: %w", err)
	}

	return r.GetNotificationPreference(ctx, req.UserID, req.ChannelType, req.TenantID)
}

func (r *PostgresRepository) GetNotificationPreference(ctx context.Context, userID, channelType, tenantID string) (NotificationPreference, error) {
	var pref NotificationPreference
	var eventTypes []string

	err := r.db.QueryRow(ctx, `
		SELECT id::text, user_id, tenant_id, channel_type, event_types, is_enabled, created_at, updated_at
		FROM notification_preferences WHERE user_id = $1 AND channel_type = $2 AND tenant_id = $3
	`, userID, channelType, tenantID).Scan(
		&pref.ID, &pref.UserID, &pref.TenantID, &pref.ChannelType, &eventTypes, &pref.IsEnabled, &pref.CreatedAt, &pref.UpdatedAt)
	if err != nil {
		return NotificationPreference{}, fmt.Errorf("failed to get notification preference: %w", err)
	}

	pref.EventTypes = eventTypes
	return pref, nil
}

func (r *PostgresRepository) ListNotificationPreferences(ctx context.Context, userID string, tenantID *string) ([]NotificationPreference, error) {
	query := `
		SELECT id::text, user_id, tenant_id, channel_type, event_types, is_enabled, created_at, updated_at
		FROM notification_preferences WHERE user_id = $1
	`
	args := []interface{}{userID}

	if tenantID != nil && *tenantID != "" {
		query += fmt.Sprintf(" AND tenant_id = $%d", len(args)+1)
		args = append(args, *tenantID)
	}

	query += " ORDER BY channel_type ASC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification preferences: %w", err)
	}
	defer rows.Close()

	result := []NotificationPreference{}
	for rows.Next() {
		var pref NotificationPreference
		var eventTypes []string

		if err := rows.Scan(
			&pref.ID, &pref.UserID, &pref.TenantID, &pref.ChannelType, &eventTypes, &pref.IsEnabled, &pref.CreatedAt, &pref.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan notification preference: %w", err)
		}

		pref.EventTypes = eventTypes
		result = append(result, pref)
	}

	return result, rows.Err()
}

func (r *PostgresRepository) UpdateNotificationPreference(ctx context.Context, id string, req UpdateNotificationPreferenceRequest) (NotificationPreference, error) {
	existing, err := r.GetNotificationPreferenceByID(ctx, id)
	if err != nil {
		return NotificationPreference{}, err
	}

	if req.EventTypes != nil {
		existing.EventTypes = *req.EventTypes
	}
	if req.IsEnabled != nil {
		existing.IsEnabled = *req.IsEnabled
	}

	now := time.Now().UTC()

	_, err = r.db.Exec(ctx, `
		UPDATE notification_preferences SET event_types = $1::text[], is_enabled = $2, updated_at = $3 WHERE id = $4
	`, existing.EventTypes, existing.IsEnabled, now, id)
	if err != nil {
		return NotificationPreference{}, fmt.Errorf("failed to update notification preference: %w", err)
	}

	return r.GetNotificationPreferenceByID(ctx, id)
}

func (r *PostgresRepository) GetNotificationPreferenceByID(ctx context.Context, id string) (NotificationPreference, error) {
	var pref NotificationPreference
	var eventTypes []string

	err := r.db.QueryRow(ctx, `
		SELECT id::text, user_id, tenant_id, channel_type, event_types, is_enabled, created_at, updated_at
		FROM notification_preferences WHERE id = $1
	`, id).Scan(
		&pref.ID, &pref.UserID, &pref.TenantID, &pref.ChannelType, &eventTypes, &pref.IsEnabled, &pref.CreatedAt, &pref.UpdatedAt)
	if err != nil {
		return NotificationPreference{}, fmt.Errorf("failed to get notification preference by ID: %w", err)
	}

	pref.EventTypes = eventTypes
	return pref, nil
}

func (r *PostgresRepository) DeleteNotificationPreference(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM notification_preferences WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete notification preference: %w", err)
	}
	return nil
}

// Utility methods

func (r *PostgresRepository) GetAlertRulesForEntity(ctx context.Context, entityType EntityType, entityID string) ([]AlertRule, error) {
	return r.ListAlertRules(ctx, nil, nil, &entityType)
}

func (r *PostgresRepository) GetChannelsForAlertRule(ctx context.Context, ruleID string) ([]NotificationChannel, error) {
	// Get the alert rule to find channel IDs
	rule, err := r.GetAlertRule(ctx, ruleID)
	if err != nil {
		return nil, err
	}

	if len(rule.NotificationChannelIDs) == 0 {
		// If no specific channels, return all active channels for the tenant
		return r.ListNotificationChannels(ctx, &rule.TenantID, nil)
	}

	// Get specific channels by IDs
	channels := make([]NotificationChannel, 0, len(rule.NotificationChannelIDs))
	for _, channelID := range rule.NotificationChannelIDs {
		ch, err := r.GetNotificationChannel(ctx, channelID)
		if err != nil {
			continue // Skip channels that don't exist
		}
		if ch.IsActive {
			channels = append(channels, ch)
		}
	}

	return channels, nil
}

// Helper function to join conditions with AND
func joinConditions(conditions []string) string {
	if len(conditions) == 0 {
		return ""
	}
	if len(conditions) == 1 {
		return conditions[0]
	}
	return "(" + joinStrings(conditions, " AND ") + ")"
}

// Helper function to join strings
func joinStrings(strings []string, sep string) string {
	if len(strings) == 0 {
		return ""
	}
	result := strings[0]
	for i := 1; i < len(strings); i++ {
		result += sep + strings[i]
	}
	return result
}
