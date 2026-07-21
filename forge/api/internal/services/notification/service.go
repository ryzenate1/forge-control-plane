package notification

import (
	"bytes"
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

type Store interface {
	CreateNotificationChannel(ctx context.Context, req store.CreateNotificationChannelRequest) (store.NotificationChannel, error)
	GetNotificationChannel(ctx context.Context, id string) (store.NotificationChannel, error)
	ListNotificationChannels(ctx context.Context) ([]store.NotificationChannel, error)
	UpdateNotificationChannel(ctx context.Context, id string, req store.UpdateNotificationChannelRequest) (store.NotificationChannel, error)
	DeleteNotificationChannel(ctx context.Context, id string) error

	CreateNotificationEventSubscription(ctx context.Context, channelID, eventType, template string) (store.NotificationEventSubscription, error)
	ListNotificationEventSubscriptions(ctx context.Context, channelID string) ([]store.NotificationEventSubscription, error)
	DeleteNotificationEventSubscription(ctx context.Context, id string) error

	CreateNotificationLog(ctx context.Context, channelID, eventType, status, errorMsg string) (store.NotificationLog, error)
	ListNotificationLogs(ctx context.Context, channelID string, limit, offset int) ([]store.NotificationLog, error)

	UpdateNotificationSubscriptionDelivery(ctx context.Context, channelID, eventType, status string) error
}

var EventTypeMapping = map[string]string{
	"server.crash":          string(events.EventServerCrashed),
	"server.install.complete": string(events.EventServerInstallCompleted),
	"backup.complete":       string(events.EventServerBackupCreated),
	"backup.failed":         string(events.EventServerBackupFailed),
	"deployment.complete":   string(events.EventDeploymentCompleted),
	"deployment.failed":     string(events.EventDeploymentFailed),
	"node.down":             string(events.EventNodeOffline),
	"node.up":               string(events.EventNodeOnline),
}

var ReverseEventTypeMapping map[string]string

func init() {
	ReverseEventTypeMapping = make(map[string]string, len(EventTypeMapping))
	for k, v := range EventTypeMapping {
		ReverseEventTypeMapping[v] = k
	}
}

type Service struct {
	store    Store
	client   *http.Client
	logger   *slog.Logger
	mu       sync.RWMutex
	channels []store.NotificationChannel
}

func New(s Store, logger *slog.Logger) *Service {
	return &Service{
		store:  s,
		client: &http.Client{Timeout: 15 * time.Second},
		logger: logger,
	}
}

func (svc *Service) RefreshChannels(ctx context.Context) error {
	channels, err := svc.store.ListNotificationChannels(ctx)
	if err != nil {
		return err
	}
	svc.mu.Lock()
	svc.channels = channels
	svc.mu.Unlock()
	return nil
}

func (svc *Service) getChannels() []store.NotificationChannel {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	result := make([]store.NotificationChannel, len(svc.channels))
	copy(result, svc.channels)
	return result
}

func (svc *Service) ListChannels(ctx context.Context) ([]store.NotificationChannel, error) {
	return svc.store.ListNotificationChannels(ctx)
}

func (svc *Service) CreateChannel(ctx context.Context, req store.CreateNotificationChannelRequest) (store.NotificationChannel, error) {
	ch, err := svc.store.CreateNotificationChannel(ctx, req)
	if err != nil {
		return store.NotificationChannel{}, err
	}
	_ = svc.RefreshChannels(ctx)
	return ch, nil
}

func (svc *Service) GetChannel(ctx context.Context, id string) (store.NotificationChannel, error) {
	return svc.store.GetNotificationChannel(ctx, id)
}

func (svc *Service) UpdateChannel(ctx context.Context, id string, req store.UpdateNotificationChannelRequest) (store.NotificationChannel, error) {
	ch, err := svc.store.UpdateNotificationChannel(ctx, id, req)
	if err != nil {
		return store.NotificationChannel{}, err
	}
	_ = svc.RefreshChannels(ctx)
	return ch, nil
}

func (svc *Service) DeleteChannel(ctx context.Context, id string) error {
	err := svc.store.DeleteNotificationChannel(ctx, id)
	if err != nil {
		return err
	}
	_ = svc.RefreshChannels(ctx)
	return nil
}

func (svc *Service) TestChannel(ctx context.Context, id string) error {
	ch, err := svc.store.GetNotificationChannel(ctx, id)
	if err != nil {
		return err
	}
	return svc.SendTest(ctx, ch)
}

func (svc *Service) ListSubscriptions(ctx context.Context, channelID string) ([]store.NotificationEventSubscription, error) {
	return svc.store.ListNotificationEventSubscriptions(ctx, channelID)
}

func (svc *Service) CreateSubscription(ctx context.Context, channelID, eventType, template string) (store.NotificationEventSubscription, error) {
	return svc.store.CreateNotificationEventSubscription(ctx, channelID, eventType, template)
}

func (svc *Service) DeleteSubscription(ctx context.Context, id string) error {
	return svc.store.DeleteNotificationEventSubscription(ctx, id)
}

func (svc *Service) ListLogs(ctx context.Context, channelID string, limit, offset int) ([]store.NotificationLog, error) {
	return svc.store.ListNotificationLogs(ctx, channelID, limit, offset)
}

func (svc *Service) Handle(ctx context.Context, ev events.Envelope) error {
	eventName := ReverseEventTypeMapping[string(ev.Type)]
	if eventName == "" {
		return nil
	}

	channels := svc.getChannels()
	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}

		subs, err := svc.store.ListNotificationEventSubscriptions(ctx, ch.ID)
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

func (svc *Service) deliver(ch store.NotificationChannel, eventName string, ev events.Envelope, template string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := svc.send(ctx, ch, eventName, ev, template)
	status := "delivered"
	errMsg := ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
		svc.logger.Error("notification delivery failed", "channel", ch.ID, "event", eventName, "error", err)
	}

	if _, storeErr := svc.store.CreateNotificationLog(ctx, ch.ID, eventName, status, errMsg); storeErr != nil {
		svc.logger.Error("create notification log", "channel", ch.ID, "error", storeErr)
	}
	if updateErr := svc.store.UpdateNotificationSubscriptionDelivery(ctx, ch.ID, eventName, status); updateErr != nil {
		svc.logger.Error("update subscription delivery", "channel", ch.ID, "error", updateErr)
	}
}

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

func (svc *Service) sendSlack(ctx context.Context, config map[string]any, eventName string, ev events.Envelope) error {
	webhookURL, _ := config["webhook_url"].(string)
	if webhookURL == "" {
		return fmt.Errorf("slack webhook_url not configured")
	}
	payload := map[string]any{
		"attachments": []any{map[string]any{
			"color": "#2196F3",
			"title": fmt.Sprintf("Event: %s", eventName),
			"text":  formatEventMessage(eventName, ev),
			"fields": []any{
				map[string]any{"title": "Resource", "value": ev.ResourceID, "short": true},
				map[string]any{"title": "Type", "value": ev.ResourceType, "short": true},
			},
			"ts": ev.Timestamp.Unix(),
		}},
	}
	return svc.postJSON(ctx, webhookURL, payload)
}

func (svc *Service) sendDiscord(ctx context.Context, config map[string]any, eventName string, ev events.Envelope) error {
	webhookURL, _ := config["webhook_url"].(string)
	if webhookURL == "" {
		return fmt.Errorf("discord webhook_url not configured")
	}
	color := 0x2196F3
	payload := map[string]any{
		"embeds": []any{map[string]any{
			"title":       fmt.Sprintf("Event: %s", eventName),
			"description": formatEventMessage(eventName, ev),
			"color":       color,
			"fields": []any{
				map[string]any{"name": "Resource", "value": ev.ResourceID, "inline": true},
				map[string]any{"name": "Type", "value": ev.ResourceType, "inline": true},
			},
			"timestamp": ev.Timestamp.Format(time.RFC3339),
		}},
	}
	return svc.postJSON(ctx, webhookURL, payload)
}

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
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	return svc.postJSON(ctx, apiURL, payload)
}

func (svc *Service) sendEmail(ctx context.Context, config map[string]any, eventName string, ev events.Envelope) error {
	recipients, _ := config["recipients"].([]any)
	if len(recipients) == 0 {
		return fmt.Errorf("email recipients not configured")
	}
	_ = recipients
	return nil
}

func (svc *Service) sendWebhook(ctx context.Context, config map[string]any, eventName string, ev events.Envelope) error {
	url, _ := config["url"].(string)
	if url == "" {
		return fmt.Errorf("webhook url not configured")
	}
	headers, _ := config["headers"].(map[string]any)
	body := map[string]any{
		"event":     eventName,
		"id":        ev.ID,
		"timestamp": ev.Timestamp,
		"source":    ev.Source,
		"resource": map[string]string{
			"type": ev.ResourceType,
			"id":   ev.ResourceID,
		},
		"payload": ev.Payload,
	}
	return svc.postJSONWithHeaders(ctx, url, body, headers)
}

func (svc *Service) postJSON(ctx context.Context, url string, payload any) error {
	return svc.postJSONWithHeaders(ctx, url, payload, nil)
}

func (svc *Service) postJSONWithHeaders(ctx context.Context, url string, payload any, headers map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GamePanel-Notification/1.0")
	for k, v := range headers {
		if s, ok := v.(string); ok {
			req.Header.Set(k, s)
		}
	}
	resp, err := svc.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notification returned status %d", resp.StatusCode)
	}
	return nil
}

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
