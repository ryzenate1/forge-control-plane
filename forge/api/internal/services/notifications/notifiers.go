package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/store"
)

// Notification is the channel-agnostic document rendered by the router and
// delivered by a Notifier.
type Notification struct {
	Event     string            `json:"event"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Severity  NotificationLevel `json:"severity"`
	Timestamp time.Time         `json:"timestamp"`
	Payload   map[string]any    `json:"payload,omitempty"`
}

// Notifier delivers a Notification over one concrete channel type. All HTTP
// notifiers are built on the shared *WebhookService (stdlib net/http, SSRF
// validated), so no new dependencies enter the tree.
type Notifier interface {
	Send(ctx context.Context, notification Notification) error
}

// MailEnqueuer is the seam to the panel's durable mail service (the same
// outbox the notification settings / triggers flows use). EmailNotifier never
// talks to SMTP directly: sends go through the retrying mail worker.
type MailEnqueuer interface {
	EnqueueMail(ctx context.Context, recipient, subject, textBody, htmlBody string) (string, error)
}

// notifierDeps carries the shared plumbing notifiers need.
type notifierDeps struct {
	http *WebhookService
	mail MailEnqueuer
}

// configString reads a trimmed string key from a channel config map.
func configString(config map[string]any, key string) string {
	value, _ := config[key].(string)
	return strings.TrimSpace(value)
}

// configStrings reads a string slice key from a channel config map, skipping
// blank entries.
func configStrings(config map[string]any, key string) []string {
	raw, _ := config[key].([]any)
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			continue
		}
		if text = strings.TrimSpace(text); text != "" {
			values = append(values, text)
		}
	}
	return values
}

// configHeaders reads a string→string header map from a channel config map.
func configHeaders(config map[string]any, key string) map[string]string {
	raw, _ := config[key].(map[string]any)
	if len(raw) == 0 {
		return nil
	}
	headers := make(map[string]string, len(raw))
	for name, value := range raw {
		text, ok := value.(string)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "host", "content-length", "connection", "transfer-encoding":
			continue
		}
		headers[name] = text
	}
	return headers
}

// --- Discord -----------------------------------------------------------

// DiscordNotifier posts rich embeds to a Discord webhook.
type DiscordNotifier struct {
	webhookURL string
	username   string
	deps       notifierDeps
}

func (n *DiscordNotifier) Send(ctx context.Context, notification Notification) error {
	embed := map[string]any{
		"title":       truncateRunes(notification.Title, 256),
		"description": truncateRunes(notification.Body, 4096),
		"color":       discordColor(notification.Severity),
		"timestamp":   notification.Timestamp.Format(time.RFC3339),
		"footer":      map[string]any{"text": "Forge · " + notification.Event},
	}
	payload := map[string]any{"embeds": []any{embed}}
	if n.username != "" {
		payload["username"] = n.username
	}
	return n.deps.http.postJSON(ctx, n.webhookURL, payload)
}

func discordColor(severity NotificationLevel) int {
	switch severity {
	case LevelCritical, LevelError:
		return 0xE74C3C
	case LevelWarning:
		return 0xE67E22
	case LevelSuccess:
		return 0x2ECC71
	default:
		return 0x3498DB
	}
}

// --- Slack -------------------------------------------------------------

// SlackNotifier posts attachment-formatted messages to a Slack webhook.
type SlackNotifier struct {
	webhookURL string
	deps       notifierDeps
}

func (n *SlackNotifier) Send(ctx context.Context, notification Notification) error {
	payload := map[string]any{
		"text": fmt.Sprintf("*%s*", escapeSlack(notification.Title)),
		"attachments": []any{map[string]any{
			"color":     slackColor(notification.Severity),
			"text":      escapeSlack(truncateRunes(notification.Body, 3500)),
			"footer":    "Forge · " + notification.Event,
			"ts":        notification.Timestamp.Unix(),
			"mrkdwn_in": []string{"text"},
		}},
	}
	return n.deps.http.postJSON(ctx, n.webhookURL, payload)
}

func slackColor(severity NotificationLevel) string {
	switch severity {
	case LevelCritical, LevelError:
		return "#e74c3c"
	case LevelWarning:
		return "#e67e22"
	case LevelSuccess:
		return "#2ecc71"
	default:
		return "#3498db"
	}
}

// escapeSlack protects Slack's own markup characters in rendered text.
func escapeSlack(text string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(text)
}

// --- Telegram ----------------------------------------------------------

// TelegramNotifier sends messages through the Telegram Bot API.
type TelegramNotifier struct {
	botToken string
	chatID   string
	deps     notifierDeps
}

func (n *TelegramNotifier) Send(ctx context.Context, notification Notification) error {
	text := fmt.Sprintf("<b>%s</b>\n\n%s\n\n<i>%s · %s</i>",
		escapeHTML(notification.Title),
		escapeHTML(truncateRunes(notification.Body, 3500)),
		escapeHTML(notification.Event),
		notification.Timestamp.Format(time.RFC3339),
	)
	payload := map[string]any{
		"chat_id":    n.chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.botToken)
	return n.deps.http.postJSON(ctx, endpoint, payload)
}

func escapeHTML(text string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(text)
}

// --- Email -------------------------------------------------------------

// EmailNotifier delivers notifications through the panel's durable mail
// outbox (one message per recipient), so SMTP retries and templating stay in
// the mail service.
type EmailNotifier struct {
	recipients []string
	deps       notifierDeps
}

func (n *EmailNotifier) Send(ctx context.Context, notification Notification) error {
	if n.deps.mail == nil {
		return fmt.Errorf("mail service is not configured")
	}
	if len(n.recipients) == 0 {
		return fmt.Errorf("email channel has no recipients configured")
	}
	subject := fmt.Sprintf("[Forge] %s", notification.Title)
	textBody := fmt.Sprintf("%s\n\n%s\n\nEvent: %s\nWhen: %s",
		notification.Title, notification.Body, notification.Event, notification.Timestamp.Format(time.RFC3339))
	var bodyParts strings.Builder
	bodyParts.WriteString("<h2>")
	bodyParts.WriteString(escapeHTML(notification.Title))
	bodyParts.WriteString("</h2><p>")
	bodyParts.WriteString(escapeHTML(notification.Body))
	bodyParts.WriteString("</p><p><small>Event ")
	bodyParts.WriteString(escapeHTML(notification.Event))
	bodyParts.WriteString(" · ")
	bodyParts.WriteString(notification.Timestamp.Format(time.RFC3339))
	bodyParts.WriteString("</small></p>")
	htmlBody := bodyParts.String()

	sent := 0
	var errs []error
	for _, recipient := range n.recipients {
		if _, err := n.deps.mail.EnqueueMail(ctx, recipient, subject, textBody, htmlBody); err != nil {
			errs = append(errs, fmt.Errorf("enqueue mail to %s: %w", recipient, err))
			continue
		}
		sent++
	}
	if sent == 0 && len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// --- Generic webhook ----------------------------------------------------

// WebhookNotifier posts the full notification document (plus payload) to a
// user-provided endpoint with optional custom headers.
type WebhookNotifier struct {
	url     string
	headers map[string]string
	deps    notifierDeps
}

func (n *WebhookNotifier) Send(ctx context.Context, notification Notification) error {
	return n.deps.http.postJSONWithHeaders(ctx, n.url, notification, n.headers)
}

// --- Factory -------------------------------------------------------------

// NewNotifier builds the Notifier for one persisted channel. It re-validates
// the channel configuration so channels created through the legacy admin
// endpoints (or corrupted rows) fail with an honest error at send time
// instead of a silent drop.
func NewNotifier(ch store.NotificationChannel, mail MailEnqueuer) (Notifier, error) {
	return newNotifierWithDeps(ch, notifierDeps{http: NewWebhookService(), mail: mail})
}

// newNotifierWithDeps is the hot-path variant used by the router so all
// deliveries share one *WebhookService (and therefore one connection pool)
// instead of building a new transport per notification.
func newNotifierWithDeps(ch store.NotificationChannel, deps notifierDeps) (Notifier, error) {
	if deps.http == nil {
		deps.http = NewWebhookService()
	}
	switch ch.Type {
	case store.NotificationChannelDiscord:
		url := configString(ch.Config, "webhook_url")
		if err := validateWebhookURL(url); err != nil {
			return nil, fmt.Errorf("discord channel %q: %w", ch.Name, err)
		}
		return &DiscordNotifier{webhookURL: url, username: configString(ch.Config, "username"), deps: deps}, nil
	case store.NotificationChannelSlack:
		url := configString(ch.Config, "webhook_url")
		if err := validateWebhookURL(url); err != nil {
			return nil, fmt.Errorf("slack channel %q: %w", ch.Name, err)
		}
		return &SlackNotifier{webhookURL: url, deps: deps}, nil
	case store.NotificationChannelTelegram:
		token := configString(ch.Config, "bot_token")
		chatID := configString(ch.Config, "chat_id")
		if token == "" || len(token) > 256 || strings.ContainsAny(token, "/?#@") {
			return nil, fmt.Errorf("telegram channel %q: invalid bot token", ch.Name)
		}
		if chatID == "" || len(chatID) > 128 {
			return nil, fmt.Errorf("telegram channel %q: invalid chat id", ch.Name)
		}
		return &TelegramNotifier{botToken: token, chatID: chatID, deps: deps}, nil
	case store.NotificationChannelEmail:
		recipients := configStrings(ch.Config, "recipients")
		if len(recipients) == 0 {
			return nil, fmt.Errorf("email channel %q: no recipients configured", ch.Name)
		}
		return &EmailNotifier{recipients: recipients, deps: deps}, nil
	case store.NotificationChannelWebhook:
		url := configString(ch.Config, "url")
		if err := validateWebhookURL(url); err != nil {
			return nil, fmt.Errorf("webhook channel %q: %w", ch.Name, err)
		}
		return &WebhookNotifier{url: url, headers: configHeaders(ch.Config, "headers"), deps: deps}, nil
	default:
		return nil, fmt.Errorf("channel %q: unsupported type %q", ch.Name, ch.Type)
	}
}

// ValidateChannelConfig checks that a create/update request carries a known
// type and a config that its notifier would accept.
func ValidateChannelConfig(channelType store.NotificationChannelType, config map[string]any) error {
	draft := store.NotificationChannel{Type: channelType, Name: "validation", Config: config}
	_, err := NewNotifier(draft, nil)
	if channelType == store.NotificationChannelEmail && err != nil && strings.Contains(err.Error(), "mail service") {
		// The email notifier defers the mail-service check to send time; a
		// recipient-only validation error above that is still real.
		return nil
	}
	return err
}

// truncateRunes shortens text to max runes, appending an ellipsis when cut.
func truncateRunes(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max-1]) + "…"
}

// jsonPreview renders payload JSON for raw webhook bodies (used by tests and
// future audit tooling); kept here so notification bodies stay comparable.
func jsonPreview(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
