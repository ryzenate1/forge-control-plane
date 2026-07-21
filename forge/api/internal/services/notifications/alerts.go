package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type AlertService struct {
	repo                Repository
	logger              *slog.Logger
	mu                  sync.RWMutex
	alertRules          []AlertRule
	eventToAlertMap     map[string][]string
	metricsCache        map[string]map[string]float64
	cacheMu             sync.RWMutex
	cooldowns           map[string]time.Time
	notificationService *Service
}

func NewAlertService(repo Repository, logger *slog.Logger, notificationService *Service) *AlertService {
	return &AlertService{
		repo:                repo,
		logger:              logger,
		alertRules:          []AlertRule{},
		eventToAlertMap:     make(map[string][]string),
		metricsCache:        make(map[string]map[string]float64),
		cooldowns:           make(map[string]time.Time),
		notificationService: notificationService,
	}
}

func (s *AlertService) LoadAlertRules(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventToAlertMap = make(map[string][]string)
	rules, err := s.repo.ListAlertRules(ctx, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to list alert rules: %w", err)
	}
	s.alertRules = rules
	for _, rule := range rules {
		if rule.EventType != nil && *rule.EventType != "" {
			s.eventToAlertMap[*rule.EventType] = append(s.eventToAlertMap[*rule.EventType], rule.ID)
		}
	}
	s.logger.Info("loaded alert rules", "count", len(rules))
	return nil
}

func (s *AlertService) RefreshAlertRules(ctx context.Context) error {
	return s.LoadAlertRules(ctx)
}

func (s *AlertService) GetAlertRule(ctx context.Context, id string) (AlertRule, error) {
	return s.repo.GetAlertRule(ctx, id)
}

func (s *AlertService) ListAlertRules(ctx context.Context, tenantID *string, userID *string, entityType *EntityType) ([]AlertRule, error) {
	return s.repo.ListAlertRules(ctx, tenantID, userID, entityType)
}

func (s *AlertService) CreateAlertRule(ctx context.Context, req CreateAlertRuleRequest) (AlertRule, error) {
	if err := req.Validate(); err != nil {
		return AlertRule{}, err
	}
	rule, err := s.repo.CreateAlertRule(ctx, req)
	if err != nil {
		return AlertRule{}, err
	}
	_ = s.RefreshAlertRules(ctx)
	return rule, nil
}

func (s *AlertService) UpdateAlertRule(ctx context.Context, id string, req UpdateAlertRuleRequest) (AlertRule, error) {
	if _, err := s.repo.GetAlertRule(ctx, id); err != nil {
		return AlertRule{}, fmt.Errorf("alert rule not found: %s", id)
	}
	rule, err := s.repo.UpdateAlertRule(ctx, id, req)
	if err != nil {
		return AlertRule{}, err
	}
	_ = s.RefreshAlertRules(ctx)
	return rule, nil
}

func (s *AlertService) DeleteAlertRule(ctx context.Context, id string) error {
	if _, err := s.repo.GetAlertRule(ctx, id); err != nil {
		return fmt.Errorf("alert rule not found: %s", id)
	}
	if err := s.repo.DeleteAlertRule(ctx, id); err != nil {
		return err
	}
	_ = s.RefreshAlertRules(ctx)
	return nil
}

func (s *AlertService) EvaluateAlert(ctx context.Context, rule *AlertRule, entityID string, currentValue *float64, currentState *string) (*AlertEvaluationResult, error) {
	state, err := 	s.repo.GetAlertState(ctx, rule.ID, entityID, rule.EntityType)
	if err != nil {
		return nil, fmt.Errorf("failed to get alert state: %w", err)
	}
	result := &AlertEvaluationResult{
		AlertRuleID: rule.ID,
		EntityID:    entityID,
		EntityType:  rule.EntityType,
		Severity:    rule.Severity,
		Message:     fmt.Sprintf("Alert evaluation for %s %s", rule.EntityType, entityID),
	}
	if currentValue != nil {
		result.CurrentValue = currentValue
	}
	if currentState != nil {
		result.CurrentState = currentState
	}
	if rule.CooldownMinutes > 0 {
		triggeredAt := state.TriggeredAt
		if triggeredAt != nil {
			cooldownEnd := triggeredAt.Add(time.Duration(rule.CooldownMinutes) * time.Minute)
			if time.Now().Before(cooldownEnd) {
				result.Message = fmt.Sprintf("Alert %s is in cooldown period until %s", rule.Name, cooldownEnd.Format(time.RFC3339))
				return result, nil
			}
		}
	}
	if rule.RuleType == AlertRuleTypeThreshold {
		return s.evaluateThresholdAlert(ctx, rule, result, &state)
	} else if rule.RuleType == AlertRuleTypeState {
		return s.evaluateStateAlert(ctx, rule, result, &state)
	} else if rule.RuleType == AlertRuleTypeEvent {
		return s.evaluateEventAlert(ctx, rule, result, &state)
	}
	return nil, fmt.Errorf("unknown alert rule type: %s", rule.RuleType)
}

func (s *AlertService) evaluateThresholdAlert(ctx context.Context, rule *AlertRule, result *AlertEvaluationResult, state *AlertState) (*AlertEvaluationResult, error) {
	if rule.MetricName == nil || rule.ThresholdValue == nil || rule.ComparisonOperator == nil {
		return result, fmt.Errorf("threshold alert missing required fields")
	}
	if result.CurrentValue == nil {
		result.Message = fmt.Sprintf("No current value available for metric %s", *rule.MetricName)
		return result, nil
	}
	threshold := *rule.ThresholdValue
	current := *result.CurrentValue
	operator := *rule.ComparisonOperator
	var conditionMet bool
	switch operator {
	case AlertOperatorGreaterThan:
		conditionMet = current > threshold
	case AlertOperatorGreaterEqual:
		conditionMet = current >= threshold
	case AlertOperatorLessThan:
		conditionMet = current < threshold
	case AlertOperatorLessEqual:
		conditionMet = current <= threshold
	case AlertOperatorEqual:
		conditionMet = current == threshold
	case AlertOperatorNotEqual:
		conditionMet = current != threshold
	default:
		return result, fmt.Errorf("unknown comparison operator: %s", operator)
	}
	if rule.DurationMinutes > 0 {
		if state != nil && state.TriggeredAt != nil {
			durationMet := time.Since(*state.TriggeredAt) >= time.Duration(rule.DurationMinutes)*time.Minute
			if !durationMet {
				result.Message = fmt.Sprintf("Condition met but duration requirement not satisfied (need %d minutes)", rule.DurationMinutes)
				return result, nil
			}
		} else {
			if conditionMet {
				now := time.Now()
				result.TriggeredAt = &now
				result.Message = fmt.Sprintf("Condition met, waiting for duration requirement (%d minutes)", rule.DurationMinutes)
				return result, nil
			}
		}
	}
	if conditionMet {
		if state != nil && state.IsActive {
			result.ShouldTrigger = false
			result.Message = fmt.Sprintf("Alert already triggered: %s", rule.Name)
		} else {
			result.ShouldTrigger = true
			result.Message = fmt.Sprintf("Alert triggered: %s (value: %f %s threshold: %f)", rule.Name, current, operator, threshold)
		}
	} else {
		if state != nil && state.IsActive {
			result.ShouldResolve = true
			result.Message = fmt.Sprintf("Alert resolved: %s (value: %f returned to normal)", rule.Name, current)
		} else {
			result.Message = fmt.Sprintf("Alert condition not met: %s (value: %f %s threshold: %f)", rule.Name, current, operator, threshold)
		}
	}
	return result, nil
}

func (s *AlertService) evaluateStateAlert(ctx context.Context, rule *AlertRule, result *AlertEvaluationResult, state *AlertState) (*AlertEvaluationResult, error) {
	if rule.StateValue == nil {
		return result, fmt.Errorf("state alert missing state value")
	}
	if result.CurrentState == nil {
		result.Message = fmt.Sprintf("No current state available for entity %s", result.EntityID)
		return result, nil
	}
	expectedState := *rule.StateValue
	currentState := *result.CurrentState
	conditionMet := currentState == expectedState
	if rule.DurationMinutes > 0 {
		if state != nil && state.TriggeredAt != nil {
			durationMet := time.Since(*state.TriggeredAt) >= time.Duration(rule.DurationMinutes)*time.Minute
			if !durationMet {
				result.Message = fmt.Sprintf("State matched but duration requirement not satisfied (need %d minutes)", rule.DurationMinutes)
				return result, nil
			}
		} else {
			if conditionMet {
				now := time.Now()
				result.TriggeredAt = &now
				result.Message = fmt.Sprintf("State matched, waiting for duration requirement (%d minutes)", rule.DurationMinutes)
				return result, nil
			}
		}
	}
	if conditionMet {
		if state != nil && state.IsActive {
			result.Message = fmt.Sprintf("Alert already triggered: %s", rule.Name)
		} else {
			result.ShouldTrigger = true
			result.Message = fmt.Sprintf("Alert triggered: %s (state: %s)", rule.Name, currentState)
		}
	} else {
		if state != nil && state.IsActive {
			result.ShouldResolve = true
			result.Message = fmt.Sprintf("Alert resolved: %s (state changed from %s)", rule.Name, expectedState)
		} else {
			result.Message = fmt.Sprintf("Alert condition not met: %s (current: %s, expected: %s)", rule.Name, currentState, expectedState)
		}
	}
	return result, nil
}

func (s *AlertService) evaluateEventAlert(ctx context.Context, rule *AlertRule, result *AlertEvaluationResult, state *AlertState) (*AlertEvaluationResult, error) {
	if rule.EventType == nil {
		return result, fmt.Errorf("event alert missing event type")
	}
	result.ShouldTrigger = true
	result.Message = fmt.Sprintf("Alert triggered by event: %s for %s %s", *rule.EventType, rule.EntityType, result.EntityID)
	return result, nil
}

func (s *AlertService) ProcessAlertResult(ctx context.Context, result *AlertEvaluationResult) error {
	rule, err := s.repo.GetAlertRule(ctx, result.AlertRuleID)
	if err != nil {
		return fmt.Errorf("failed to get alert rule: %w", err)
	}
	state, err := s.repo.GetAlertState(ctx, result.AlertRuleID, result.EntityID, result.EntityType)
	if err != nil {
		return fmt.Errorf("failed to get alert state: %w", err)
	}
	var cval *float64
	var cstate *string
	if result.ShouldTrigger {
		cval = result.CurrentValue
	} else if result.ShouldResolve {
		cstate = nil
	}
	_, err = s.repo.UpdateAlertState(ctx, state.ID, cval, cstate, result.ShouldTrigger)
	if err != nil {
		return fmt.Errorf("failed to update alert state: %w", err)
	}
	if result.ShouldTrigger {
		now := time.Now()
		updateReq := UpdateAlertRuleRequest{LastTriggeredAt: &now}
		_, err = s.repo.UpdateAlertRule(ctx, result.AlertRuleID, updateReq)
		if err != nil {
			s.logger.Error("failed to update alert rule last triggered time", "error", err)
		}
		go s.sendAlertNotifications(ctx, &rule, result)
	}
	return nil
}

func (s *AlertService) sendAlertNotifications(ctx context.Context, rule *AlertRule, result *AlertEvaluationResult) {
	channels, err := s.repo.GetChannelsForAlertRule(ctx, rule.ID)
	if err != nil {
		s.logger.Error("failed to get notification channels for alert", "alert_rule_id", rule.ID, "error", err)
		return
	}
	message := result.Message
	if rule.Description != nil && *rule.Description != "" {
		message = fmt.Sprintf("%s: %s", *rule.Description, message)
	}
	for _, channel := range channels {
		if !channel.IsActive {
			continue
		}
		notificationMsg := NotificationMessage{
			ChannelID:   channel.ID,
			AlertRuleID: &rule.ID,
			EventType:   "alert.triggered",
			Title:       fmt.Sprintf("Alert: %s", rule.Name),
			Message:     message,
			Severity:    rule.Severity,
			Payload: map[string]interface{}{
				"alert_rule_id":   rule.ID,
				"alert_rule_name": rule.Name,
				"entity_id":       result.EntityID,
				"entity_type":     string(result.EntityType),
				"message":         message,
				"severity":        string(rule.Severity),
				"timestamp":       time.Now().UTC().Format(time.RFC3339),
			},
			Timestamp: time.Now().UTC(),
			TenantID:  rule.TenantID,
		}
		if err := s.notificationService.SendToChannel(ctx, channel, message, notificationMsg.Payload); err != nil {
			s.logger.Error("failed to send alert notification", "alert_rule_id", rule.ID, "channel_id", channel.ID, "error", err)
			logEntry := NotificationLog{
				ID:           GenerateID(),
				TenantID:     rule.TenantID,
				ChannelID:    channel.ID,
				AlertRuleID:  &rule.ID,
				EventType:    "alert.triggered",
				Status:       NotificationLogStatusFailed,
				ErrorMessage: stringPtr(err.Error()),
				Payload:      map[string]interface{}{"alert_rule_id": rule.ID, "message": message},
				SentAt:       time.Now().UTC(),
			}
			_, _ = s.repo.CreateNotificationLog(ctx, logEntry)
		} else {
			logEntry := NotificationLog{
				ID:          GenerateID(),
				TenantID:    rule.TenantID,
				ChannelID:   channel.ID,
				AlertRuleID: &rule.ID,
				EventType:   "alert.triggered",
				Status:      NotificationLogStatusDelivered,
				Payload:     map[string]interface{}{"alert_rule_id": rule.ID, "message": message},
				SentAt:      time.Now().UTC(),
			}
			_, _ = s.repo.CreateNotificationLog(ctx, logEntry)
		}
	}
}

func (s *AlertService) EvaluateEntity(ctx context.Context, entityType EntityType, entityID string, metrics map[string]float64, state *string) ([]*AlertEvaluationResult, error) {
	var results []*AlertEvaluationResult
	rules, err := s.repo.ListAlertRules(ctx, nil, nil, &entityType)
	if err != nil {
		return nil, fmt.Errorf("failed to list alert rules: %w", err)
	}
	for _, rule := range rules {
		var currentValue *float64
		if rule.MetricName != nil {
			if val, exists := metrics[*rule.MetricName]; exists {
				currentValue = &val
			}
		}
		result, err := s.EvaluateAlert(ctx, &rule, entityID, currentValue, state)
		if err != nil {
			s.logger.Error("failed to evaluate alert", "alert_rule_id", rule.ID, "error", err)
			continue
		}
		if err := s.ProcessAlertResult(ctx, result); err != nil {
			s.logger.Error("failed to process alert result", "alert_rule_id", rule.ID, "error", err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *AlertService) EvaluateEvent(ctx context.Context, eventType, resourceType, resourceID string, payload map[string]interface{}) ([]*AlertEvaluationResult, error) {
	var results []*AlertEvaluationResult
	var ruleIDs []string
	s.mu.RLock()
	if ids, exists := s.eventToAlertMap[eventType]; exists {
		ruleIDs = ids
	}
	s.mu.RUnlock()
	et := EntityType(resourceType)
	rules, err := s.repo.ListAlertRules(ctx, nil, nil, &et)
	if err != nil {
		return nil, fmt.Errorf("failed to list alert rules: %w", err)
	}
	ruleIDSet := make(map[string]bool)
	for _, id := range ruleIDs {
		ruleIDSet[id] = true
	}
	for _, rule := range rules {
		if rule.EventType != nil && *rule.EventType == eventType {
			ruleIDSet[rule.ID] = true
		}
	}
	var allRules []AlertRule
	for id := range ruleIDSet {
		rule, err := s.repo.GetAlertRule(ctx, id)
		if err != nil {
			s.logger.Error("failed to get alert rule", "id", id, "error", err)
			continue
		}
		allRules = append(allRules, rule)
	}
	for _, rule := range allRules {
		if rule.RuleType == AlertRuleTypeEvent && (rule.EventType == nil || *rule.EventType != eventType) {
			continue
		}
		var currentValue *float64
		var currentState *string
		if rule.RuleType == AlertRuleTypeThreshold && rule.MetricName != nil {
			if val, exists := payload[*rule.MetricName]; exists {
				if fval, ok := val.(float64); ok {
					currentValue = &fval
				}
			}
		}
		if rule.RuleType == AlertRuleTypeState && rule.StateValue != nil {
			if val, exists := payload["state"]; exists {
				if sval, ok := val.(string); ok {
					currentState = &sval
				}
			}
		}
		result, err := s.EvaluateAlert(ctx, &rule, resourceID, currentValue, currentState)
		if err != nil {
			s.logger.Error("failed to evaluate alert", "alert_rule_id", rule.ID, "error", err)
			continue
		}
		if err := s.ProcessAlertResult(ctx, result); err != nil {
			s.logger.Error("failed to process alert result", "alert_rule_id", rule.ID, "error", err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *AlertService) UpdateMetrics(entityType, entityID string, metrics map[string]float64) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if _, exists := s.metricsCache[entityType]; !exists {
		s.metricsCache[entityType] = make(map[string]float64)
	}
	for key, value := range metrics {
		s.metricsCache[entityType][entityID+":"+key] = value
	}
}

func (s *AlertService) GetMetric(entityType, entityID, metricName string) (float64, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if entityMetrics, exists := s.metricsCache[entityType]; exists {
		if value, exists := entityMetrics[entityID+":"+metricName]; exists {
			return value, true
		}
	}
	return 0, false
}

func boolPtr(b bool) *bool { return &b }
func stringPtr(s string) *string { return &s }
