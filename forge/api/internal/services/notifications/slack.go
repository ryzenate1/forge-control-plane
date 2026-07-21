package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SlackService handles Slack notifications
type SlackService struct {
	client   *http.Client
	baseURL string
}

// NewSlackService creates a new Slack service
func NewSlackService() *SlackService {
	return &SlackService{
		client:   &http.Client{Timeout: 15 * time.Second},
		baseURL: "https://hooks.slack.com/services",
	}
}

// SlackMessage represents a message to be sent to Slack
type SlackMessage struct {
	Channel     string            `json:"channel,omitempty"`
	Username    string            `json:"username,omitempty"`
	IconEmoji   string            `json:"icon_emoji,omitempty"`
	Text        string            `json:"text,omitempty"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
	Blocks      []SlackBlock      `json:"blocks,omitempty"`
}

// SlackAttachment represents a Slack message attachment
type SlackAttachment struct {
	Color      string       `json:"color,omitempty"`
	Fallback   string       `json:"fallback,omitempty"`
	Title      string       `json:"title,omitempty"`
	Text       string       `json:"text,omitempty"`
	Fields     []SlackField `json:"fields,omitempty"`
	Timestamp  int64        `json:"ts,omitempty"`
	Footer     string       `json:"footer,omitempty"`
	FooterIcon string       `json:"footer_icon,omitempty"`
}

// SlackField represents a field in a Slack attachment
type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// SlackBlock represents a Slack message block
type SlackBlock struct {
	Type     string      `json:"type"`
	Text     *SlackText  `json:"text,omitempty"`
	Fields   []SlackField `json:"fields,omitempty"`
	Elements []interface{} `json:"elements,omitempty"`
	ImageURL string      `json:"image_url,omitempty"`
	AltText  string      `json:"alt_text,omitempty"`
}

// SlackText represents text in a Slack block
type SlackText struct {
	Type  string `json:"type"` // "plain_text" or "mrkdwn"
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

// SlackResponse represents the response from Slack API
type SlackResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Send sends a message to Slack
func (s *SlackService) Send(ctx context.Context, webhookURL string, message SlackMessage) error {
	// Marshal the message
	body, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create Slack request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GamePanel-Notification/1.0")

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Slack request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Slack API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to check for errors
	var slackResp SlackResponse
	if err := json.NewDecoder(resp.Body).Decode(&slackResp); err != nil {
		// If we can't parse the response, but status was 200, it's probably fine
		return nil
	}

	if !slackResp.OK {
		return fmt.Errorf("Slack API error: %s", slackResp.Error)
	}

	return nil
}

// SendSimpleMessage sends a simple text message to Slack
func (s *SlackService) SendSimpleMessage(ctx context.Context, webhookURL, text string) error {
	return s.Send(ctx, webhookURL, SlackMessage{
		Text: text,
	})
}

// GetColorForSeverity returns the appropriate color for a given severity
func GetColorForSeverity(severity AlertSeverity) string {
	switch severity {
	case SeverityCritical, SeverityError:
		return "#ff0000"
	case SeverityWarning:
		return "#ffcc00"
	case SeverityInfo:
		return "#36a64f"
	default:
		return "#2196F3"
	}
}

// SendAlert sends an alert notification to Slack
func (s *SlackService) SendAlert(ctx context.Context, webhookURL string, alert AlertEvaluationResult, message string) error {
	// Determine color based on severity
	color := "#2196F3" // Default blue
	if alert.Severity == AlertSeverityWarning {
		color = "#FFC107" // Amber
	} else if alert.Severity == AlertSeverityCritical || alert.Severity == AlertSeverityEmergency {
		color = "#F44336" // Red
	}

	// Create attachment
	attachment := SlackAttachment{
		Color:     color,
		Fallback:  message,
		Title:     fmt.Sprintf("Alert: %s", message),
		Text:      message,
		Timestamp: time.Now().Unix(),
		Fields: []SlackField{
			{Title: "Entity", Value: alert.EntityID, Short: true},
			{Title: "Type", Value: string(alert.EntityType), Short: true},
			{Title: "Status", Value: "Triggered", Short: true},
		},
		Footer: "GamePanel Notification System",
	}

	return s.Send(ctx, webhookURL, SlackMessage{
		Attachments: []SlackAttachment{attachment},
	})
}

// SendEventNotification sends an event notification to Slack
func (s *SlackService) SendEventNotification(ctx context.Context, webhookURL, eventType, resourceID, resourceType, message string) error {
	// Determine color based on event type
	color := getSlackColorForEvent(eventType)

	// Create attachment
	attachment := SlackAttachment{
		Color:     color,
		Fallback:  fmt.Sprintf("%s: %s", eventType, resourceID),
		Title:     fmt.Sprintf("Event: %s", eventType),
		Text:      message,
		Timestamp: time.Now().Unix(),
		Fields: []SlackField{
			{Title: "Resource", Value: resourceID, Short: true},
			{Title: "Type", Value: resourceType, Short: true},
		},
		Footer: "GamePanel Notification System",
	}

	return s.Send(ctx, webhookURL, SlackMessage{
		Attachments: []SlackAttachment{attachment},
	})
}

// getSlackColorForEvent returns a color for the given event type
func getSlackColorForEvent(eventType string) string {
	switch {
	case strings.Contains(eventType, "failed"),
		strings.Contains(eventType, "crash"),
		strings.Contains(eventType, "down"),
		strings.Contains(eventType, "error"):
		return "#F44336" // Red
	case strings.Contains(eventType, "warning"):
		return "#FFC107" // Amber
	case strings.Contains(eventType, "complete"),
		strings.Contains(eventType, "success"),
		strings.Contains(eventType, "up"),
		strings.Contains(eventType, "online"):
		return "#4CAF50" // Green
	default:
		return "#2196F3" // Blue
	}
}

// SendTestNotification sends a test notification to Slack
func (s *SlackService) SendTestNotification(ctx context.Context, webhookURL string) error {
	attachment := SlackAttachment{
		Color:     "#4CAF50",
		Fallback:  "Test notification from GamePanel",
		Title:     "Test Notification",
		Text:      "This is a test notification from GamePanel to verify that your Slack webhook is working correctly.",
		Timestamp: time.Now().Unix(),
		Fields: []SlackField{
			{Title: "Status", Value: "✅ Working", Short: true},
			{Title: "Timestamp", Value: time.Now().Format(time.RFC3339), Short: true},
		},
		Footer: "GamePanel Notification System",
	}

	return s.Send(ctx, webhookURL, SlackMessage{
		Attachments: []SlackAttachment{attachment},
	})
}

// SendRichNotification sends a rich notification with blocks to Slack
func (s *SlackService) SendRichNotification(ctx context.Context, webhookURL, title, message string, fields []SlackField) error {
	// Create blocks for a richer notification
	blocks := []SlackBlock{
		{
			Type: "header",
			Text: &SlackText{
				Type: "plain_text",
				Text: title,
				Emoji: true,
			},
		},
		{
			Type: "section",
			Text: &SlackText{
				Type: "mrkdwn",
				Text: message,
			},
		},
	}

	// Add fields if provided
	if len(fields) > 0 {
		// Group fields into pairs (Slack allows max 10 fields per section)
		for i := 0; i < len(fields); i += 2 {
			end := i + 2
			if end > len(fields) {
				end = len(fields)
			}
			fieldGroup := fields[i:end]

			blocks = append(blocks, SlackBlock{
				Type:   "section",
				Fields: fieldGroup,
			})
		}
	}

	blocks = append(blocks, SlackBlock{
		Type: "context",
		Elements: []interface{}{
			map[string]string{
				"type": "mrkdwn",
				"text": fmt.Sprintf("Sent at: %s", time.Now().Format(time.RFC3339)),
			},
		},
	})

	return s.Send(ctx, webhookURL, SlackMessage{
		Blocks: blocks,
	})
}

// ValidateWebhookURL validates a Slack webhook URL
func (s *SlackService) ValidateWebhookURL(webhookURL string) error {
	// Basic validation
	if webhookURL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	// Check if it starts with the expected base URL
	if !strings.HasPrefix(webhookURL, "https://hooks.slack.com/services/") {
		// Allow custom Slack-compatible endpoints
		if !strings.Contains(webhookURL, "hooks.slack.com") && !strings.Contains(webhookURL, "slack.com") {
			return fmt.Errorf("invalid Slack webhook URL format")
		}
	}

	return nil
}
