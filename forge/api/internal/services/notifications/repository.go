package notifications

import (
	"context"
)

// Repository defines the interface for notification data persistence
type Repository interface {
	// Notification Channel operations
	CreateNotificationChannel(ctx context.Context, req CreateNotificationChannelRequest) (NotificationChannel, error)
	GetNotificationChannel(ctx context.Context, id string) (NotificationChannel, error)
	ListNotificationChannels(ctx context.Context, tenantID *string, userID *string) ([]NotificationChannel, error)
	UpdateNotificationChannel(ctx context.Context, id string, req UpdateNotificationChannelRequest) (NotificationChannel, error)
	DeleteNotificationChannel(ctx context.Context, id string) error

	// Alert Rule operations
	CreateAlertRule(ctx context.Context, req CreateAlertRuleRequest) (AlertRule, error)
	GetAlertRule(ctx context.Context, id string) (AlertRule, error)
	ListAlertRules(ctx context.Context, tenantID *string, userID *string, entityType *EntityType) ([]AlertRule, error)
	UpdateAlertRule(ctx context.Context, id string, req UpdateAlertRuleRequest) (AlertRule, error)
	DeleteAlertRule(ctx context.Context, id string) error
	GetAlertRulesForEntity(ctx context.Context, entityType EntityType, entityID string) ([]AlertRule, error)

	// Alert State operations
	CreateAlertState(ctx context.Context, ruleID, entityID string, entityType EntityType, currentValue *float64, currentState *string) (AlertState, error)
	GetAlertState(ctx context.Context, ruleID, entityID string, entityType EntityType) (AlertState, error)
	UpdateAlertState(ctx context.Context, id string, currentValue *float64, currentState *string, isActive bool) (AlertState, error)
	ListActiveAlertStates(ctx context.Context, tenantID *string) ([]AlertState, error)
	GetAlertStateByID(ctx context.Context, id string) (AlertState, error)

	// Notification Log operations
	CreateNotificationLog(ctx context.Context, log NotificationLog) (NotificationLog, error)
	ListNotificationLogs(ctx context.Context, tenantID *string, channelID *string, limit, offset int) ([]NotificationLog, error)
	GetNotificationLog(ctx context.Context, id string) (NotificationLog, error)

	// Notification Preference operations
	CreateNotificationPreference(ctx context.Context, req CreateNotificationPreferenceRequest) (NotificationPreference, error)
	GetNotificationPreference(ctx context.Context, userID, channelType, tenantID string) (NotificationPreference, error)
	ListNotificationPreferences(ctx context.Context, userID string, tenantID *string) ([]NotificationPreference, error)
	UpdateNotificationPreference(ctx context.Context, id string, req UpdateNotificationPreferenceRequest) (NotificationPreference, error)
	DeleteNotificationPreference(ctx context.Context, id string) error

	// Channel operations
	GetChannelsForAlertRule(ctx context.Context, ruleID string) ([]NotificationChannel, error)
}
