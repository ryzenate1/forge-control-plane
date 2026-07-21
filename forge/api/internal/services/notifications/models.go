package notifications

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// NotificationChannelType represents the type of notification channel
type NotificationChannelType string

const (
	NotificationChannelSlack    NotificationChannelType = "slack"
	NotificationChannelDiscord  NotificationChannelType = "discord"
	NotificationChannelTelegram NotificationChannelType = "telegram"
	NotificationChannelEmail    NotificationChannelType = "email"
	NotificationChannelWebhook  NotificationChannelType = "webhook"
	NotificationChannelSMS      NotificationChannelType = "sms"
)

// ChannelConfig represents the configuration for a specific channel type
type ChannelConfig struct {
	// Slack configuration
	SlackWebhookURL string `json:"slack_webhook_url,omitempty"`

	// Discord configuration
	DiscordWebhookURL string `json:"discord_webhook_url,omitempty"`

	// Telegram configuration
	TelegramBotToken string `json:"telegram_bot_token,omitempty"`
	TelegramChatID   string `json:"telegram_chat_id,omitempty"`

	// Email configuration
	EmailRecipients []string `json:"email_recipients,omitempty"`
	EmailSMTPHost   string   `json:"email_smtp_host,omitempty"`
	EmailSMTPPort   int      `json:"email_smtp_port,omitempty"`
	EmailSMTPUser   string   `json:"email_smtp_user,omitempty"`
	EmailSMTPPass   string   `json:"email_smtp_pass,omitempty"`
	EmailFrom       string   `json:"email_from,omitempty"`
	EmailUseTLS     bool     `json:"email_use_tls,omitempty"`

	// Webhook configuration
	WebhookURL     string            `json:"webhook_url,omitempty"`
	WebhookHeaders map[string]string `json:"webhook_headers,omitempty"`

	// SMS configuration
	SMSProvider    string `json:"sms_provider,omitempty"`
	SMSPhoneNumber string `json:"sms_phone_number,omitempty"`
	SMSAPIKey      string `json:"sms_api_key,omitempty"`
}

// NotificationChannel represents a notification channel
type NotificationChannel struct {
	ID          string                  `json:"id" db:"id"`
	TenantID    string                  `json:"tenant_id" db:"tenant_id"`
	UserID      *string                 `json:"user_id,omitempty" db:"user_id"`
	Type        NotificationChannelType `json:"type" db:"type"`
	Name        string                  `json:"name" db:"name"`
	Description *string                 `json:"description,omitempty" db:"description"`
	Config      ChannelConfig           `json:"config" db:"config"`
	IsActive    bool                    `json:"is_active" db:"is_active"`
	CreatedAt   time.Time               `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at" db:"updated_at"`
}

// CreateNotificationChannelRequest represents a request to create a notification channel
type CreateNotificationChannelRequest struct {
	Type        NotificationChannelType `json:"type"`
	Name        string                  `json:"name"`
	Description *string                 `json:"description,omitempty"`
	Config      ChannelConfig           `json:"config"`
	IsActive    bool                    `json:"is_active,omitempty"`
	TenantID    string                  `json:"tenant_id,omitempty"`
	UserID      *string                 `json:"user_id,omitempty"`
}

// UpdateNotificationChannelRequest represents a request to update a notification channel
type UpdateNotificationChannelRequest struct {
	Name        *string        `json:"name,omitempty"`
	Description *string        `json:"description,omitempty"`
	Config      *ChannelConfig `json:"config,omitempty"`
	IsActive    *bool          `json:"is_active,omitempty"`
}

// AlertRuleType represents the type of alert rule
type AlertRuleType string

const (
	AlertRuleTypeThreshold AlertRuleType = "threshold"
	AlertRuleTypeState     AlertRuleType = "state"
	AlertRuleTypeEvent     AlertRuleType = "event"
)

// AlertSeverity represents the severity level of an alert
type AlertSeverity string

const (
	AlertSeverityInfo      AlertSeverity = "info"
	AlertSeverityWarning   AlertSeverity = "warning"
	AlertSeverityCritical  AlertSeverity = "critical"
	AlertSeverityEmergency AlertSeverity = "emergency"
)

// AlertComparisonOperator represents comparison operators for threshold alerts
type AlertComparisonOperator string

const (
	AlertOperatorGreaterThan  AlertComparisonOperator = ">"
	AlertOperatorGreaterEqual AlertComparisonOperator = ">="
	AlertOperatorLessThan     AlertComparisonOperator = "<"
	AlertOperatorLessEqual    AlertComparisonOperator = "<="
	AlertOperatorEqual        AlertComparisonOperator = "=="
	AlertOperatorNotEqual     AlertComparisonOperator = "!="
)

// MetricName represents a metric name type
type MetricName string

// Short operator aliases used by the alert engine
const (
	OperatorGreaterThan  AlertComparisonOperator = ">"
	OperatorLessThan     AlertComparisonOperator = "<"
	OperatorGreaterEqual AlertComparisonOperator = ">="
	OperatorLessEqual    AlertComparisonOperator = "<="
	OperatorEqual        AlertComparisonOperator = "=="
	OperatorNotEqual     AlertComparisonOperator = "!="
)

// Short aliases for notification log status values
const StatusPending NotificationLogStatus = NotificationLogStatusPending
const StatusFailed NotificationLogStatus = NotificationLogStatusFailed
const StatusDelivered NotificationLogStatus = NotificationLogStatusDelivered

// EntityType represents the type of entity being monitored
type EntityType string

const (
	EntityTypeServer     EntityType = "server"
	EntityTypeNode       EntityType = "node"
	EntityTypeBackup     EntityType = "backup"
	EntityTypeDeployment EntityType = "deployment"
	EntityTypeDatabase   EntityType = "database"
	EntityTypeApp        EntityType = "app"
	EntityTypeVolume     EntityType = "volume"
	EntityTypeNetwork    EntityType = "network"
)

// AlertRule represents an alert rule that triggers notifications
type AlertRule struct {
	ID                     string                   `json:"id" db:"id"`
	TenantID               string                   `json:"tenant_id" db:"tenant_id"`
	UserID                 *string                  `json:"user_id,omitempty" db:"user_id"`
	Name                   string                   `json:"name" db:"name"`
	Description            *string                  `json:"description,omitempty" db:"description"`
	RuleType               AlertRuleType            `json:"rule_type" db:"rule_type"`
	EntityType             EntityType               `json:"entity_type" db:"entity_type"`
	MetricName             *string                  `json:"metric_name,omitempty" db:"metric_name"`
	ThresholdValue         *float64                 `json:"threshold_value,omitempty" db:"threshold_value"`
	ComparisonOperator     *AlertComparisonOperator `json:"comparison_operator,omitempty" db:"comparison_operator"`
	StateValue             *string                  `json:"state_value,omitempty" db:"state_value"`
	EventType              *string                  `json:"event_type,omitempty" db:"event_type"`
	DurationMinutes        int                      `json:"duration_minutes" db:"duration_minutes"`
	CooldownMinutes        int                      `json:"cooldown_minutes" db:"cooldown_minutes"`
	Severity               AlertSeverity            `json:"severity" db:"severity"`
	IsEnabled              bool                     `json:"is_enabled" db:"is_enabled"`
	NotificationChannelIDs []string                 `json:"notification_channel_ids" db:"notification_channel_ids"`
	Conditions             json.RawMessage          `json:"conditions,omitempty" db:"conditions"`
	CreatedAt              time.Time                `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time                `json:"updated_at" db:"updated_at"`
	LastTriggeredAt        *time.Time               `json:"last_triggered_at,omitempty" db:"last_triggered_at"`
}

// CreateAlertRuleRequest represents a request to create an alert rule
type CreateAlertRuleRequest struct {
	Name                   string                   `json:"name"`
	Description            *string                  `json:"description,omitempty"`
	RuleType               AlertRuleType            `json:"rule_type"`
	EntityType             EntityType               `json:"entity_type"`
	MetricName             *string                  `json:"metric_name,omitempty"`
	ThresholdValue         *float64                 `json:"threshold_value,omitempty"`
	ComparisonOperator     *AlertComparisonOperator `json:"comparison_operator,omitempty"`
	StateValue             *string                  `json:"state_value,omitempty"`
	EventType              *string                  `json:"event_type,omitempty"`
	DurationMinutes        int                      `json:"duration_minutes,omitempty"`
	CooldownMinutes        int                      `json:"cooldown_minutes,omitempty"`
	Severity               AlertSeverity            `json:"severity,omitempty"`
	IsEnabled              bool                     `json:"is_enabled,omitempty"`
	NotificationChannelIDs []string                 `json:"notification_channel_ids,omitempty"`
	Conditions             map[string]interface{}   `json:"conditions,omitempty"`
	TenantID               string                   `json:"tenant_id,omitempty"`
	UserID                 *string                  `json:"user_id,omitempty"`
}

// UpdateAlertRuleRequest represents a request to update an alert rule
type UpdateAlertRuleRequest struct {
	Name                   *string                  `json:"name,omitempty"`
	Description            *string                  `json:"description,omitempty"`
	RuleType               *AlertRuleType           `json:"rule_type,omitempty"`
	EntityType             *EntityType              `json:"entity_type,omitempty"`
	MetricName             *string                  `json:"metric_name,omitempty"`
	ThresholdValue         *float64                 `json:"threshold_value,omitempty"`
	ComparisonOperator     *AlertComparisonOperator `json:"comparison_operator,omitempty"`
	StateValue             *string                  `json:"state_value,omitempty"`
	EventType              *string                  `json:"event_type,omitempty"`
	DurationMinutes        *int                     `json:"duration_minutes,omitempty"`
	CooldownMinutes        *int                     `json:"cooldown_minutes,omitempty"`
	Severity               *AlertSeverity           `json:"severity,omitempty"`
	IsEnabled              *bool                    `json:"is_enabled,omitempty"`
	NotificationChannelIDs *[]string                `json:"notification_channel_ids,omitempty"`
	Conditions             *map[string]interface{}  `json:"conditions,omitempty"`
	LastTriggeredAt        *time.Time               `json:"last_triggered_at,omitempty"`
}

// AlertState represents the current state of an alert for a specific entity
type AlertState struct {
	ID           string     `json:"id" db:"id"`
	AlertRuleID  string     `json:"alert_rule_id" db:"alert_rule_id"`
	EntityID     string     `json:"entity_id" db:"entity_id"`
	EntityType   EntityType `json:"entity_type" db:"entity_type"`
	CurrentValue *float64   `json:"current_value,omitempty" db:"current_value"`
	CurrentState *string    `json:"current_state,omitempty" db:"current_state"`
	TriggeredAt  *time.Time `json:"triggered_at,omitempty" db:"triggered_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty" db:"resolved_at"`
	IsActive     bool       `json:"is_active" db:"is_active"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

// NotificationLogStatus represents the status of a notification delivery
type NotificationLogStatus string

const (
	NotificationLogStatusPending   NotificationLogStatus = "pending"
	NotificationLogStatusDelivered NotificationLogStatus = "delivered"
	NotificationLogStatusFailed    NotificationLogStatus = "failed"
)

// NotificationLog represents a log entry for notification delivery
type NotificationLog struct {
	ID           string                 `json:"id" db:"id"`
	TenantID     string                 `json:"tenant_id" db:"tenant_id"`
	ChannelID    string                 `json:"channel_id" db:"channel_id"`
	AlertRuleID  *string                `json:"alert_rule_id,omitempty" db:"alert_rule_id"`
	EventType    string                 `json:"event_type" db:"event_type"`
	Status       NotificationLogStatus  `json:"status" db:"status"`
	ErrorMessage *string                `json:"error_message,omitempty" db:"error_message"`
	Payload      map[string]interface{} `json:"payload,omitempty" db:"payload"`
	SentAt       time.Time              `json:"sent_at" db:"sent_at"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
}

// CreateNotificationLogRequest represents a request to create a notification log
type CreateNotificationLogRequest struct {
	ChannelID    string                 `json:"channel_id"`
	AlertRuleID  *string                `json:"alert_rule_id,omitempty"`
	EventType    string                 `json:"event_type"`
	Status       NotificationLogStatus  `json:"status"`
	ErrorMessage *string                `json:"error_message,omitempty"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
	TenantID     string                 `json:"tenant_id,omitempty"`
}

// NotificationPreferences represents user notification preferences
type NotificationPreferences struct {
	ID          string    `json:"id" db:"id"`
	UserID      string    `json:"user_id" db:"user_id"`
	TenantID    string    `json:"tenant_id" db:"tenant_id"`
	ChannelType string    `json:"channel_type" db:"channel_type"`
	EventTypes  []string  `json:"event_types" db:"event_types"`
	IsEnabled   bool      `json:"is_enabled" db:"is_enabled"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// CreateNotificationPreferencesRequest represents a request to create notification preferences
type CreateNotificationPreferencesRequest struct {
	UserID      string   `json:"user_id"`
	TenantID    string   `json:"tenant_id,omitempty"`
	ChannelType string   `json:"channel_type"`
	EventTypes  []string `json:"event_types,omitempty"`
	IsEnabled   bool     `json:"is_enabled,omitempty"`
}

// UpdateNotificationPreferencesRequest represents a request to update notification preferences
type UpdateNotificationPreferencesRequest struct {
	EventTypes *[]string `json:"event_types,omitempty"`
	IsEnabled  *bool     `json:"is_enabled,omitempty"`
}

// AlertEvaluationResult represents the result of evaluating an alert rule
type AlertEvaluationResult struct {
	AlertRuleID   string        `json:"alert_rule_id"`
	EntityID      string        `json:"entity_id"`
	EntityType    EntityType    `json:"entity_type"`
	Severity      AlertSeverity `json:"severity"`
	CurrentValue  *float64      `json:"current_value,omitempty"`
	CurrentState  *string       `json:"current_state,omitempty"`
	ShouldTrigger bool          `json:"should_trigger"`
	ShouldResolve bool          `json:"should_resolve"`
	TriggeredAt   *time.Time    `json:"triggered_at,omitempty"`
	Message       string        `json:"message"`
}

// NotificationMessage represents a message to be sent through a notification channel
type NotificationMessage struct {
	ChannelID   string                 `json:"channel_id"`
	AlertRuleID *string                `json:"alert_rule_id,omitempty"`
	EventType   string                 `json:"event_type"`
	Title       string                 `json:"title"`
	Message     string                 `json:"message"`
	Severity    AlertSeverity          `json:"severity"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	TenantID    string                 `json:"tenant_id"`
}

// ChannelFilter represents filters for listing notification channels
type ChannelFilter struct {
	TenantID *string                  `json:"tenant_id,omitempty"`
	UserID   *string                  `json:"user_id,omitempty"`
	Type     *NotificationChannelType `json:"type,omitempty"`
	IsActive *bool                    `json:"is_active,omitempty"`
	Search   *string                  `json:"search,omitempty"`
	Page     int                      `json:"page,omitempty"`
	PerPage  int                      `json:"per_page,omitempty"`
}

// AlertRuleFilter represents filters for listing alert rules
type AlertRuleFilter struct {
	TenantID   *string        `json:"tenant_id,omitempty"`
	UserID     *string        `json:"user_id,omitempty"`
	EntityType *EntityType    `json:"entity_type,omitempty"`
	IsEnabled  *bool          `json:"is_enabled,omitempty"`
	Severity   *AlertSeverity `json:"severity,omitempty"`
	Search     *string        `json:"search,omitempty"`
	Page       int            `json:"page,omitempty"`
	PerPage    int            `json:"per_page,omitempty"`
}

// LogFilter represents filters for listing notification logs
type LogFilter struct {
	TenantID    *string                `json:"tenant_id,omitempty"`
	ChannelID   *string                `json:"channel_id,omitempty"`
	AlertRuleID *string                `json:"alert_rule_id,omitempty"`
	EventType   *string                `json:"event_type,omitempty"`
	Status      *NotificationLogStatus `json:"status,omitempty"`
	StartDate   *time.Time             `json:"start_date,omitempty"`
	EndDate     *time.Time             `json:"end_date,omitempty"`
	Page        int                    `json:"page,omitempty"`
	PerPage     int                    `json:"per_page,omitempty"`
}

// GenerateID generates a new UUID for models
func GenerateID() string {
	return uuid.NewString()
}

// GetChannelConfig returns the configuration for a specific channel type
func (c *NotificationChannel) GetChannelConfig() ChannelConfig {
	return c.Config
}

// SetChannelConfig sets the configuration for a channel
func (c *NotificationChannel) SetChannelConfig(config ChannelConfig) {
	c.Config = config
}

// Validate validates the notification channel
func (c *CreateNotificationChannelRequest) Validate() error {
	if c.Type == "" {
		return &ValidationError{Field: "type", Message: "channel type is required"}
	}
	if c.Name == "" {
		return &ValidationError{Field: "name", Message: "channel name is required"}
	}

	// Validate channel-specific configuration
	if err := c.Config.Validate(c.Type); err != nil {
		return err
	}

	return nil
}

// Validate validates the channel configuration based on type
type AlertEvent struct {
	ID           string                 `json:"id"`
	AlertRuleID  string                 `json:"alert_rule_id"`
	EntityID     string                 `json:"entity_id"`
	EntityType   EntityType             `json:"entity_type"`
	MetricName   *string                `json:"metric_name,omitempty"`
	CurrentValue *float64               `json:"current_value,omitempty"`
	CurrentState *string                `json:"current_state,omitempty"`
	Severity     AlertSeverity          `json:"severity"`
	Message      string                 `json:"message"`
	Timestamp    time.Time              `json:"timestamp"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

type NotificationEventSubscription struct {
	ID             string
	ChannelID      string
	EventType      string
	Template       string
	LastSentAt     *time.Time
	DeliveryStatus string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// WebhookConfig represents webhook configuration
type WebhookConfig struct {
	URL     string
	Method  string
	Headers map[string]string
}

type CreateNotificationPreferenceRequest struct {
	UserID      string
	TenantID    string
	ChannelType string
	EventTypes  []string
	IsEnabled   bool
}

type NotificationPreference struct {
	ID          string
	UserID      string
	TenantID    string
	ChannelType string
	EventTypes  []string
	IsEnabled   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UpdateNotificationPreferenceRequest struct {
	EventTypes *[]string
	IsEnabled  *bool
}

func (c *ChannelConfig) Validate(channelType NotificationChannelType) error {
	switch channelType {
	case NotificationChannelSlack:
		if c.SlackWebhookURL == "" {
			return &ValidationError{Field: "slack_webhook_url", Message: "Slack webhook URL is required"}
		}
	case NotificationChannelDiscord:
		if c.DiscordWebhookURL == "" {
			return &ValidationError{Field: "discord_webhook_url", Message: "Discord webhook URL is required"}
		}
	case NotificationChannelTelegram:
		if c.TelegramBotToken == "" {
			return &ValidationError{Field: "telegram_bot_token", Message: "Telegram bot token is required"}
		}
		if c.TelegramChatID == "" {
			return &ValidationError{Field: "telegram_chat_id", Message: "Telegram chat ID is required"}
		}
	case NotificationChannelEmail:
		if len(c.EmailRecipients) == 0 {
			return &ValidationError{Field: "email_recipients", Message: "at least one email recipient is required"}
		}
		if c.EmailSMTPHost == "" {
			return &ValidationError{Field: "email_smtp_host", Message: "SMTP host is required"}
		}
		if c.EmailFrom == "" {
			return &ValidationError{Field: "email_from", Message: "from email address is required"}
		}
	case NotificationChannelWebhook:
		if c.WebhookURL == "" {
			return &ValidationError{Field: "webhook_url", Message: "webhook URL is required"}
		}
	case NotificationChannelSMS:
		if c.SMSPhoneNumber == "" {
			return &ValidationError{Field: "sms_phone_number", Message: "phone number is required"}
		}
		if c.SMSAPIKey == "" {
			return &ValidationError{Field: "sms_api_key", Message: "SMS API key is required"}
		}
	}

	return nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// Validate validates the alert rule
func (r *CreateAlertRuleRequest) Validate() error {
	if r.Name == "" {
		return &ValidationError{Field: "name", Message: "alert rule name is required"}
	}
	if r.RuleType == "" {
		return &ValidationError{Field: "rule_type", Message: "rule type is required"}
	}
	if r.EntityType == "" {
		return &ValidationError{Field: "entity_type", Message: "entity type is required"}
	}

	// Validate rule type specific fields
	if r.RuleType == AlertRuleTypeThreshold {
		if r.MetricName == nil || *r.MetricName == "" {
			return &ValidationError{Field: "metric_name", Message: "metric name is required for threshold alerts"}
		}
		if r.ThresholdValue == nil {
			return &ValidationError{Field: "threshold_value", Message: "threshold value is required for threshold alerts"}
		}
		if r.ComparisonOperator == nil || *r.ComparisonOperator == "" {
			return &ValidationError{Field: "comparison_operator", Message: "comparison operator is required for threshold alerts"}
		}
	}

	if r.RuleType == AlertRuleTypeState {
		if r.StateValue == nil || *r.StateValue == "" {
			return &ValidationError{Field: "state_value", Message: "state value is required for state alerts"}
		}
	}

	if r.RuleType == AlertRuleTypeEvent {
		if r.EventType == nil || *r.EventType == "" {
			return &ValidationError{Field: "event_type", Message: "event type is required for event alerts"}
		}
	}

	if r.DurationMinutes <= 0 {
		r.DurationMinutes = 5 // Default
	}
	if r.CooldownMinutes <= 0 {
		r.CooldownMinutes = 30 // Default
	}
	if r.Severity == "" {
		r.Severity = AlertSeverityInfo // Default
	}

	return nil
}

// Severity constants used throughout the notifications package
const (
	SeverityCritical AlertSeverity = "critical"
	SeverityError    AlertSeverity = "error"
	SeverityWarning  AlertSeverity = "warning"
	SeverityInfo     AlertSeverity = "info"
)
