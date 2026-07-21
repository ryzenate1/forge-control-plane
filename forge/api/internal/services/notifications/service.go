package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

// Service provides notification functionality
type Service struct {
	logger             *slog.Logger
	repository         Repository
	emailService       *EmailService
	slackService       *SlackService
	webhookService     *WebhookService
	alertService        *AlertService
	notificationStore  store.Store
	mu                 sync.RWMutex
	channels           []store.NotificationChannel
	eventSubscriptions map[string][]store.NotificationEventSubscription
}

// New creates a new notification service
func New(logger *slog.Logger, repository Repository, s store.Store) *Service {
	svc := &Service{
		logger:             logger,
		repository:         repository,
		emailService:       NewEmailService(EmailConfig{}),
		slackService:       NewSlackService(),
		webhookService:     NewWebhookService(),
		notificationStore:  s,
		channels:           make([]store.NotificationChannel, 0),
		eventSubscriptions: make(map[string][]store.NotificationEventSubscription),
	}

	// Initialize alert service
	svc.alertService = NewAlertService(repository, logger, svc)

	return svc
}

// Start starts the notification service
func (svc *Service) Start(ctx context.Context) {
	svc.logger.Info("starting notification service")

	// Load channels
	if err := svc.RefreshChannels(ctx); err != nil {
		svc.logger.Error("failed to load notification channels", "error", err)
	}

	// Load subscriptions
	if err := svc.RefreshSubscriptions(ctx); err != nil {
		svc.logger.Error("failed to load notification subscriptions", "error", err)
	}

	// Start alert service
	if err := svc.alertService.LoadAlertRules(ctx); err != nil {
		svc.logger.Error("failed to load alert rules", "error", err)
	}

	svc.logger.Info("notification service started")
}

// RefreshChannels refreshes the list of notification channels
func (svc *Service) RefreshChannels(ctx context.Context) error {
	channels, err := svc.notificationStore.ListNotificationChannels(ctx)
	if err != nil {
		return err
	}

	svc.mu.Lock()
	svc.channels = channels
	svc.mu.Unlock()

	svc.logger.Info("refreshed notification channels", "count", len(channels))
	return nil
}

// getChannels returns the current list of channels
func (svc *Service) getChannels() []store.NotificationChannel {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	result := make([]store.NotificationChannel, len(svc.channels))
	copy(result, svc.channels)
	return result
}

// RefreshSubscriptions refreshes the event subscriptions
func (svc *Service) RefreshSubscriptions(ctx context.Context) error {
	channels := svc.getChannels()

	svc.mu.Lock()
	svc.eventSubscriptions = make(map[string][]store.NotificationEventSubscription)
	svc.mu.Unlock()

	for _, channel := range channels {
		subs, err := svc.notificationStore.ListNotificationEventSubscriptions(ctx, channel.ID)
		if err != nil {
			svc.logger.Error("failed to load subscriptions for channel", "channel", channel.ID, "error", err)
			continue
		}

		svc.mu.Lock()
		svc.eventSubscriptions[channel.ID] = subs
		svc.mu.Unlock()
	}

	svc.logger.Info("refreshed notification subscriptions")
	return nil
}

// Handle handles incoming events and routes them to appropriate channels
func (svc *Service) Handle(ctx context.Context, ev events.Envelope) error {
	eventName := ReverseEventTypeMapping[string(ev.Type)]
	if eventName == "" {
		// Try to handle as a direct event type
		eventName = string(ev.Type)
	}

	channels := svc.getChannels()
	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}

		subs, err := svc.notificationStore.ListNotificationEventSubscriptions(ctx, ch.ID)
		if err != nil {
			svc.logger.Error("list subscriptions", "channel", ch.ID, "error", err)
			continue
		}

		matched := false
		var template string
		for _, sub := range subs {
			if sub.EventType == eventName {
				matched = true
				template = sub.Template
				break
			}
		}
		if !matched {
			continue
		}

		go svc.deliver(ch, eventName, ev, template)
	}
	return nil
}

// deliver delivers a notification to a specific channel
func (svc *Service) deliver(ch store.NotificationChannel, eventName string, ev events.Envelope, template string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := svc.send(ctx, ch, eventName, ev, template)
	status := "delivered"
	var errMsg string
	if err != nil {
		status = "failed"
		errMsg = err.Error()
		svc.logger.Error("notification delivery failed", "channel", ch.ID, "event", eventName, "error", err)
	}

	if _, storeErr := svc.notificationStore.CreateNotificationLog(ctx, ch.ID, eventName, status, errMsg); storeErr != nil {
		svc.logger.Error("create notification log", "channel", ch.ID, "error", storeErr)
	}
	if updateErr := svc.notificationStore.UpdateNotificationSubscriptionDelivery(ctx, ch.ID, eventName, status); updateErr != nil {
		svc.logger.Error("update subscription delivery", "channel", ch.ID, "error", updateErr)
	}
}

// send sends a notification to a specific channel
func (svc *Service) send(ctx context.Context, ch store.NotificationChannel, eventName string, ev events.Envelope, template string) error {
	switch ch.Type {
	case store.NotificationChannelSlack:
		return svc.sendSlack(ctx, ch.Config, eventName, ev)
	case store.NotificationChannelDiscord:
		return svc.sendDiscord(ctx, ch.Config, eventName, ev)
	case store.NotificationChannelTelegram:
		return svc.sendTelegram(ctx, ch.Config, eventName, ev)
	case store.NotificationChannelEmail:
		return svc.sendEmail(ctx, ch.Config, eventName, ev)
	case store.NotificationChannelWebhook:
		return svc.sendWebhook(ctx, ch.Config, eventName, ev)
	default:
		return fmt.Errorf("unknown channel type: %s", ch.Type)
	}
}

// configToMap converts a ChannelConfig struct to a generic map for send functions
func configToMap(cfg ChannelConfig) map[string]any {
	data, _ := json.Marshal(cfg)
	var m map[string]any
	json.Unmarshal(data, &m)
	return m
}

// SendToChannel sends a notification to a specific channel
func (svc *Service) SendToChannel(ctx context.Context, ch NotificationChannel, message string, payload interface{}) error {
	cfg := configToMap(ch.Config)
	switch ch.Type {
	case NotificationChannelSlack:
		return svc.sendSlackMessage(ctx, cfg, message)
	case NotificationChannelDiscord:
		return svc.sendDiscordMessage(ctx, cfg, message)
	case NotificationChannelTelegram:
		return svc.sendTelegramMessage(ctx, cfg, message)
	case NotificationChannelEmail:
		return svc.sendEmailMessage(ctx, cfg, message)
	case NotificationChannelWebhook:
		return svc.sendWebhookMessage(ctx, cfg, payload)
	default:
		return fmt.Errorf("unknown channel type: %s", ch.Type)
	}
}

// sendSlack sends a Slack notification
func (svc *Service) sendSlack(ctx context.Context, config map[string]any, eventName string, ev events.Envelope) error {
	webhookURL, _ := config["webhook_url"].(string)
	if webhookURL == "" {
		return fmt.Errorf("slack webhook_url not configured")
	}

	message := formatEventMessage(eventName, ev)
	color := "#2196F3"

	// Determine color based on event type
	switch eventName {
	case "server.crash", "backup.failed", "deployment.failed", "node.down":
		color = "#ff0000"
	case "backup.complete", "deployment.complete", "node.up":
		color = "#28a745"
	case "server.install.complete":
		color = "#17a2b8"
	}

	return svc.slackService.Send(ctx, webhookURL, SlackMessage{Text: message, Attachments: []SlackAttachment{{Color: color}}})
}

// sendSlackMessage sends a simple Slack message
func (svc *Service) sendSlackMessage(ctx context.Context, config map[string]any, message string) error {
	webhookURL, _ := config["webhook_url"].(string)
	if webhookURL == "" {
		return fmt.Errorf("slack webhook_url not configured")
	}
	return svc.slackService.SendSimpleMessage(ctx, webhookURL, message)
}

// sendDiscord sends a Discord notification
func (svc *Service) sendDiscord(ctx context.Context, config map[string]any, eventName string, ev events.Envelope) error {
	webhookURL, _ := config["webhook_url"].(string)
	if webhookURL == "" {
		return fmt.Errorf("discord webhook_url not configured")
	}

	color := 0x2196F3
	message := formatEventMessage(eventName, ev)

	// Determine color based on event type
	switch eventName {
	case "server.crash", "backup.failed", "deployment.failed", "node.down":
		color = 0xDC3545
	case "backup.complete", "deployment.complete", "node.up":
		color = 0x28A745
	case "server.install.complete":
		color = 0x17A2B8
	}

	payload := map[string]any{
		"embeds": []any{map[string]any{
			"title":       fmt.Sprintf("Event: %s", eventName),
			"description": message,
			"color":       color,
			"fields": []any{
				map[string]any{"name": "Resource", "value": ev.ResourceID, "inline": true},
				map[string]any{"name": "Type", "value": ev.ResourceType, "inline": true},
			},
			"timestamp": ev.Timestamp.Format(time.RFC3339),
		}},
	}

	_, err := svc.webhookService.SendSimple(ctx, WebhookConfig{
		URL:    webhookURL,
		Method: http.MethodPost,
	}, payload)
	return err
}

// sendDiscordMessage sends a simple Discord message
func (svc *Service) sendDiscordMessage(ctx context.Context, config map[string]any, message string) error {
	webhookURL, _ := config["webhook_url"].(string)
	if webhookURL == "" {
		return fmt.Errorf("discord webhook_url not configured")
	}

	payload := map[string]any{
		"content": message,
	}

	_, err := svc.webhookService.SendSimple(ctx, WebhookConfig{
		URL:    webhookURL,
		Method: http.MethodPost,
	}, payload)
	return err
}

// sendTelegram sends a Telegram notification
func (svc *Service) sendTelegram(ctx context.Context, config map[string]any, eventName string, ev events.Envelope) error {
	botToken, _ := config["bot_token"].(string)
	chatID, _ := config["chat_id"].(string)
	if botToken == "" || chatID == "" {
		return fmt.Errorf("telegram bot_token and chat_id required")
	}

	text := fmt.Sprintf("*%s*\n%s\n\nResource: `%s` (%s)", eventName, formatEventMessage(eventName, ev), ev.ResourceID, ev.ResourceType)
	if len(ev.Payload) > 0 {
		details, _ := json.MarshalIndent(ev.Payload, "", "  ")
		text += "\n\n```json\n" + string(details) + "\n```"
	}

	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	_, err := svc.webhookService.SendSimple(ctx, WebhookConfig{
		URL:    fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken),
		Method: http.MethodPost,
	}, payload)
	return err
}

// sendTelegramMessage sends a simple Telegram message
func (svc *Service) sendTelegramMessage(ctx context.Context, config map[string]any, message string) error {
	botToken, _ := config["bot_token"].(string)
	chatID, _ := config["chat_id"].(string)
	if botToken == "" || chatID == "" {
		return fmt.Errorf("telegram bot_token and chat_id required")
	}

	payload := map[string]any{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "Markdown",
	}

	_, err := svc.webhookService.SendSimple(ctx, WebhookConfig{
		URL:    fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken),
		Method: http.MethodPost,
	}, payload)
	return err
}

// sendEmail sends an email notification
func (svc *Service) sendEmail(ctx context.Context, config map[string]any, eventName string, ev events.Envelope) error {
	recipients, _ := config["recipients"].([]any)
	if len(recipients) == 0 {
		return fmt.Errorf("email recipients not configured")
	}

	// Convert recipients to string slice
	stringRecipients := make([]string, len(recipients))
	for i, recipient := range recipients {
		if str, ok := recipient.(string); ok {
			stringRecipients[i] = str
		}
	}

	if len(stringRecipients) == 0 {
		return fmt.Errorf("no valid email recipients")
	}

	subject := fmt.Sprintf("[GamePanel] %s", eventName)
	body := fmt.Sprintf("<h1>%s</h1><p>%s</p><p>Resource: %s (%s)</p><p>Timestamp: %s</p>",
		eventName,
		formatEventMessage(eventName, ev),
		ev.ResourceID,
		ev.ResourceType,
		ev.Timestamp.Format(time.RFC3339))

	return svc.emailService.Send(ctx, EmailMessage{
		To:      stringRecipients,
		Subject: subject,
		Body:    body,
		IsHTML:  true,
	})
}

// sendEmailMessage sends a simple email message
func (svc *Service) sendEmailMessage(ctx context.Context, config map[string]any, message string) error {
	recipients, _ := config["recipients"].([]any)
	if len(recipients) == 0 {
		return fmt.Errorf("email recipients not configured")
	}

	// Convert recipients to string slice
	stringRecipients := make([]string, len(recipients))
	for i, recipient := range recipients {
		if str, ok := recipient.(string); ok {
			stringRecipients[i] = str
		}
	}

	if len(stringRecipients) == 0 {
		return fmt.Errorf("no valid email recipients")
	}

	subject := config["subject"].(string)
	if subject == "" {
		subject = "GamePanel Notification"
	}

	return svc.emailService.Send(ctx, EmailMessage{To: stringRecipients, Subject: subject, Body: message})
}

// sendWebhook sends a webhook notification
func (svc *Service) sendWebhook(ctx context.Context, config map[string]any, eventName string, ev events.Envelope) error {
	url, _ := config["url"].(string)
	if url == "" {
		return fmt.Errorf("webhook url not configured")
	}

	headers, _ := config["headers"].(map[string]any)
	stringHeaders := make(map[string]string)
	for k, v := range headers {
		if str, ok := v.(string); ok {
			stringHeaders[k] = str
		}
	}

	msg := WebhookMessage{
		Event:     eventName,
		ID:        ev.ID,
		Timestamp: ev.Timestamp,
		Source:    ev.Source,
		Resource:  map[string]string{"type": ev.ResourceType, "id": ev.ResourceID},
		Payload:   ev.Payload,
	}

	_, err := svc.webhookService.Send(ctx, WebhookConfig{
		URL:     url,
		Method:  http.MethodPost,
		Headers: stringHeaders,
	}, msg)
	return err
}

// sendWebhookMessage sends a webhook message with custom payload
func (svc *Service) sendWebhookMessage(ctx context.Context, config map[string]any, payload interface{}) error {
	url, _ := config["url"].(string)
	if url == "" {
		return fmt.Errorf("webhook url not configured")
	}

	headers, _ := config["headers"].(map[string]any)
	stringHeaders := make(map[string]string)
	for k, v := range headers {
		if str, ok := v.(string); ok {
			stringHeaders[k] = str
		}
	}

	_, err := svc.webhookService.SendSimple(ctx, WebhookConfig{
		URL:     url,
		Method:  http.MethodPost,
		Headers: stringHeaders,
	}, payload)
	return err
}

// SendTest sends a test notification to a channel
func (svc *Service) SendTest(ctx context.Context, ch store.NotificationChannel) error {
	ev := events.NewEnvelope(
		events.EventType("test.notification"),
		"notification-service",
		"test",
		"",
		map[string]any{"message": "This is a test notification from GamePanel"},
	)
	return svc.send(ctx, ch, "test.notification", ev, "")
}

// TestChannel tests a notification channel
func (svc *Service) TestChannel(ctx context.Context, id string) error {
	ch, err := svc.notificationStore.GetNotificationChannel(ctx, id)
	if err != nil {
		return err
	}
	return svc.SendTest(ctx, ch)
}

// ListChannels lists all notification channels
func (svc *Service) ListChannels(ctx context.Context) ([]store.NotificationChannel, error) {
	return svc.notificationStore.ListNotificationChannels(ctx)
}

// CreateChannel creates a new notification channel
func (svc *Service) CreateChannel(ctx context.Context, req store.CreateNotificationChannelRequest) (store.NotificationChannel, error) {
	ch, err := svc.notificationStore.CreateNotificationChannel(ctx, req)
	if err != nil {
		return store.NotificationChannel{}, err
	}
	_ = svc.RefreshChannels(ctx)
	return ch, nil
}

// GetChannel gets a notification channel by ID
func (svc *Service) GetChannel(ctx context.Context, id string) (store.NotificationChannel, error) {
	return svc.notificationStore.GetNotificationChannel(ctx, id)
}

// UpdateChannel updates a notification channel
func (svc *Service) UpdateChannel(ctx context.Context, id string, req store.UpdateNotificationChannelRequest) (store.NotificationChannel, error) {
	ch, err := svc.notificationStore.UpdateNotificationChannel(ctx, id, req)
	if err != nil {
		return store.NotificationChannel{}, err
	}
	_ = svc.RefreshChannels(ctx)
	return ch, nil
}

// DeleteChannel deletes a notification channel
func (svc *Service) DeleteChannel(ctx context.Context, id string) error {
	err := svc.notificationStore.DeleteNotificationChannel(ctx, id)
	if err != nil {
		return err
	}
	_ = svc.RefreshChannels(ctx)
	return nil
}

// ListSubscriptions lists subscriptions for a channel
func (svc *Service) ListSubscriptions(ctx context.Context, channelID string) ([]store.NotificationEventSubscription, error) {
	return svc.notificationStore.ListNotificationEventSubscriptions(ctx, channelID)
}

// CreateSubscription creates a new subscription
func (svc *Service) CreateSubscription(ctx context.Context, channelID, eventType, template string) (store.NotificationEventSubscription, error) {
	return svc.notificationStore.CreateNotificationEventSubscription(ctx, channelID, eventType, template)
}

// DeleteSubscription deletes a subscription
func (svc *Service) DeleteSubscription(ctx context.Context, id string) error {
	return svc.notificationStore.DeleteNotificationEventSubscription(ctx, id)
}

// ListLogs lists notification logs
func (svc *Service) ListLogs(ctx context.Context, channelID string, limit, offset int) ([]store.NotificationLog, error) {
	return svc.notificationStore.ListNotificationLogs(ctx, channelID, limit, offset)
}

// UpdateSubscriptionDelivery updates the delivery status of a subscription
func (svc *Service) UpdateSubscriptionDelivery(ctx context.Context, channelID, eventType, status string) error {
	return svc.notificationStore.UpdateNotificationSubscriptionDelivery(ctx, channelID, eventType, status)
}

// EventTypeMapping maps internal event types to notification event types
var EventTypeMapping = map[string]string{
	"server.crash":            string(events.EventServerCrashed),
	"server.install.complete": string(events.EventServerInstallCompleted),
	"backup.complete":         string(events.EventServerBackupCreated),
	"backup.failed":           string(events.EventServerBackupFailed),
	"deployment.complete":     string(events.EventDeploymentCompleted),
	"deployment.failed":       string(events.EventDeploymentFailed),
	"node.down":               string(events.EventNodeOffline),
	"node.up":                 string(events.EventNodeOnline),
}

// ReverseEventTypeMapping maps event types back to notification event names
var ReverseEventTypeMapping map[string]string

func init() {
	ReverseEventTypeMapping = make(map[string]string, len(EventTypeMapping))
	for k, v := range EventTypeMapping {
		ReverseEventTypeMapping[v] = k
	}
}

// formatEventMessage formats an event message for notifications
func formatEventMessage(eventName string, ev events.Envelope) string {
	switch eventName {
	case "server.crash":
		return fmt.Sprintf("Server %s has crashed", ev.ResourceID)
	case "server.install.complete":
		return fmt.Sprintf("Server %s installation completed", ev.ResourceID)
	case "backup.complete":
		return fmt.Sprintf("Backup completed for server %s", ev.ResourceID)
	case "backup.failed":
		return fmt.Sprintf("Backup failed for server %s", ev.ResourceID)
	case "deployment.complete":
		return fmt.Sprintf("Deployment completed for %s %s", ev.ResourceType, ev.ResourceID)
	case "deployment.failed":
		return fmt.Sprintf("Deployment failed for %s %s", ev.ResourceType, ev.ResourceID)
	case "node.down":
		return fmt.Sprintf("Node %s is offline", ev.ResourceID)
	case "node.up":
		return fmt.Sprintf("Node %s is back online", ev.ResourceID)
	default:
		return fmt.Sprintf("Event %s for %s %s", eventName, ev.ResourceType, ev.ResourceID)
	}
}

// Alert Engine Methods

// EvaluateThreshold evaluates threshold-based alert rules
func (svc *Service) EvaluateThreshold(ctx context.Context, entityType EntityType, entityID, metricName string, currentValue float64) {
	if svc.alertService != nil {
		metrics := map[string]float64{metricName: currentValue}
		_, _ = svc.alertService.EvaluateEntity(ctx, entityType, entityID, metrics, nil)
	}
}

// EvaluateState evaluates state-based alert rules
func (svc *Service) EvaluateState(ctx context.Context, entityType EntityType, entityID, stateValue string) {
	if svc.alertService != nil {
		svc.alertService.EvaluateEvent(ctx, string(entityType), string(entityType), entityID, nil)
	}
}

// GetAlertService returns the alert service
func (svc *Service) GetAlertService() *AlertService {
	return svc.alertService
}

// Alert Rule Methods

// ListAlertRules lists alert rules with optional filtering
func (svc *Service) ListAlertRules(ctx context.Context, tenantID *string, userID *string, entityType *EntityType) ([]AlertRule, error) {
	return svc.repository.ListAlertRules(ctx, tenantID, userID, entityType)
}

// CreateAlertRule creates a new alert rule
func (svc *Service) CreateAlertRule(ctx context.Context, req CreateAlertRuleRequest) (AlertRule, error) {
	return svc.repository.CreateAlertRule(ctx, req)
}

// GetAlertRule gets an alert rule by ID
func (svc *Service) GetAlertRule(ctx context.Context, id string) (AlertRule, error) {
	return svc.repository.GetAlertRule(ctx, id)
}

// UpdateAlertRule updates an alert rule
func (svc *Service) UpdateAlertRule(ctx context.Context, id string, req UpdateAlertRuleRequest) (AlertRule, error) {
	return svc.repository.UpdateAlertRule(ctx, id, req)
}

// DeleteAlertRule deletes an alert rule
func (svc *Service) DeleteAlertRule(ctx context.Context, id string) error {
	return svc.repository.DeleteAlertRule(ctx, id)
}

// ListActiveAlertStates lists active alert states
func (svc *Service) ListActiveAlertStates(ctx context.Context, tenantID *string) ([]AlertState, error) {
	return svc.repository.ListActiveAlertStates(ctx, tenantID)
}

// Notification Preference Methods

// CreateNotificationPreference creates a notification preference
func (svc *Service) CreateNotificationPreference(ctx context.Context, req CreateNotificationPreferenceRequest) (NotificationPreference, error) {
	return svc.repository.CreateNotificationPreference(ctx, req)
}

// GetNotificationPreference gets a notification preference
func (svc *Service) GetNotificationPreference(ctx context.Context, userID, channelType, tenantID string) (NotificationPreference, error) {
	return svc.repository.GetNotificationPreference(ctx, userID, channelType, tenantID)
}

// ListNotificationPreferences lists notification preferences for a user
func (svc *Service) ListNotificationPreferences(ctx context.Context, userID string, tenantID *string) ([]NotificationPreference, error) {
	return svc.repository.ListNotificationPreferences(ctx, userID, tenantID)
}

// UpdateNotificationPreference updates a notification preference
func (svc *Service) UpdateNotificationPreference(ctx context.Context, id string, req UpdateNotificationPreferenceRequest) (NotificationPreference, error) {
	return svc.repository.UpdateNotificationPreference(ctx, id, req)
}

// DeleteNotificationPreference deletes a notification preference
func (svc *Service) DeleteNotificationPreference(ctx context.Context, id string) error {
	return svc.repository.DeleteNotificationPreference(ctx, id)
}
