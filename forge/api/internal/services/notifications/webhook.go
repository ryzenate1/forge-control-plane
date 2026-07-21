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

// WebhookService handles generic HTTP webhook notifications
type WebhookService struct {
	client *http.Client
}

// NewWebhookService creates a new webhook service
func NewWebhookService() *WebhookService {
	return &WebhookService{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// WebhookMessage represents a message to be sent to a webhook
type WebhookMessage struct {
	Event     string                 `json:"event"`
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source"`
	Resource  map[string]string      `json:"resource"`
	Payload   map[string]interface{} `json:"payload"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// WebhookResponse represents the response from a webhook endpoint
type WebhookResponse struct {
	StatusCode int
	Body       string
	Headers    http.Header
}

// Send sends a message to a webhook
func (s *WebhookService) Send(ctx context.Context, config WebhookConfig, message WebhookMessage) (*WebhookResponse, error) {
	// Set default method
	if config.Method == "" {
		config.Method = http.MethodPost
	}

	// Marshal the message
	body, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal webhook message: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, config.Method, config.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GamePanel-Notification/1.0")
	req.Header.Set("X-GamePanel-Event", message.Event)
	req.Header.Set("X-GamePanel-Timestamp", message.Timestamp.Format(time.RFC3339))

	// Add custom headers
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send webhook request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, _ := io.ReadAll(resp.Body)

	return &WebhookResponse{
		StatusCode: resp.StatusCode,
		Body:       string(respBody),
		Headers:    resp.Header,
	}, nil
}

// SendSimple sends a simple JSON payload to a webhook
func (s *WebhookService) SendSimple(ctx context.Context, config WebhookConfig, payload interface{}) (*WebhookResponse, error) {
	// Create a simple message
	message := WebhookMessage{
		Event:     "notification",
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		Timestamp: time.Now().UTC(),
		Source:    "gamepanel",
		Resource:  map[string]string{},
		Payload:   map[string]interface{}{},
	}

	// If payload is a map, use it directly
	if payloadMap, ok := payload.(map[string]interface{}); ok {
		message.Payload = payloadMap
	} else {
		// Marshal and unmarshal to convert to map
		data, _ := json.Marshal(payload)
		json.Unmarshal(data, &message.Payload)
	}

	return s.Send(ctx, config, message)
}

// SendAlert sends an alert notification to a webhook
func (s *WebhookService) SendAlert(ctx context.Context, config WebhookConfig, alert AlertEvaluationResult, message string) (*WebhookResponse, error) {
	payload := map[string]interface{}{
		"alert_id":       alert.AlertRuleID,
		"entity_id":      alert.EntityID,
		"entity_type":    string(alert.EntityType),
		"message":        message,
		"should_trigger": alert.ShouldTrigger,
		"should_resolve": alert.ShouldResolve,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	}

	if alert.CurrentValue != nil {
		payload["current_value"] = *alert.CurrentValue
	}
	if alert.CurrentState != nil {
		payload["current_state"] = *alert.CurrentState
	}

	return s.SendSimple(ctx, config, map[string]interface{}{
		"event":    "alert",
		"severity": string(alert.Severity),
		"data":     payload,
	})
}

// SendEvent sends an event notification to a webhook
func (s *WebhookService) SendEvent(ctx context.Context, config WebhookConfig, eventType, resourceID, resourceType, message string) (*WebhookResponse, error) {
	payload := map[string]interface{}{
		"event_type":    eventType,
		"resource_id":   resourceID,
		"resource_type": resourceType,
		"message":       message,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}

	return s.SendSimple(ctx, config, map[string]interface{}{
		"event": eventType,
		"data":  payload,
	})
}

// SendTestNotification sends a test notification to a webhook
func (s *WebhookService) SendTestNotification(ctx context.Context, config WebhookConfig) (*WebhookResponse, error) {
	return s.SendSimple(ctx, config, map[string]interface{}{
		"test":        true,
		"message":     "This is a test notification from GamePanel",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"source":      "gamepanel",
		"description": "Test notification to verify webhook configuration",
	})
}

// ValidateURL validates a webhook URL
func (s *WebhookService) ValidateURL(url string) error {
	if url == "" {
		return fmt.Errorf("webhook URL is required")
	}

	// Basic URL validation
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("webhook URL must start with http:// or https://")
	}

	// Parse the URL to check if it's valid
	parsed, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	if parsed.URL.Scheme == "" || parsed.URL.Host == "" {
		return fmt.Errorf("invalid webhook URL format")
	}

	return nil
}

// RetryPolicy defines retry behavior for webhook deliveries
type RetryPolicy struct {
	MaxRetries        int
	RetryInterval     time.Duration
	BackoffMultiplier float64
}

// DefaultRetryPolicy returns the default retry policy
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:        3,
		RetryInterval:     1 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// SendWithRetry sends a webhook message with retry logic
func (s *WebhookService) SendWithRetry(ctx context.Context, config WebhookConfig, message WebhookMessage, policy RetryPolicy) (*WebhookResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		// Add attempt info to headers
		if config.Headers == nil {
			config.Headers = make(map[string]string)
		}
		config.Headers["X-GamePanel-Attempt"] = fmt.Sprintf("%d", attempt+1)

		resp, err := s.Send(ctx, config, message)
		if err != nil {
			lastErr = err
			if attempt < policy.MaxRetries {
				// Wait before retrying
				waitTime := policy.RetryInterval * time.Duration(policy.BackoffMultiplier)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(waitTime):
					// Continue to next attempt
				}
				continue
			}
			return nil, err
		}

		// Check if we should retry based on status code
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			// Server error or rate limiting - retry
			if attempt < policy.MaxRetries {
				waitTime := policy.RetryInterval * time.Duration(policy.BackoffMultiplier)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(waitTime):
					// Continue to next attempt
				}
				continue
			}
		}

		// Success or client error (4xx) - don't retry
		return resp, nil
	}

	return nil, lastErr
}

// WebhookDeliveryResult represents the result of a webhook delivery attempt
type WebhookDeliveryResult struct {
	Success    bool
	StatusCode int
	Error      string
	Attempts   int
	TotalTime  time.Duration
}

// SendWithMetrics sends a webhook with retry and returns delivery metrics
func (s *WebhookService) SendWithMetrics(ctx context.Context, config WebhookConfig, message WebhookMessage, policy RetryPolicy) (*WebhookDeliveryResult, error) {
	startTime := time.Now()

	result := &WebhookDeliveryResult{
		Success:   false,
		Attempts:  0,
		TotalTime: 0,
	}

	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		result.Attempts++

		// Add attempt info
		if config.Headers == nil {
			config.Headers = make(map[string]string)
		}
		config.Headers["X-GamePanel-Attempt"] = fmt.Sprintf("%d", attempt+1)

		resp, err := s.Send(ctx, config, message)
		if err != nil {
			result.Error = err.Error()
			if attempt < policy.MaxRetries {
				waitTime := policy.RetryInterval * time.Duration(policy.BackoffMultiplier)
				select {
				case <-ctx.Done():
					result.TotalTime = time.Since(startTime)
					return result, ctx.Err()
				case <-time.After(waitTime):
					// Continue
				}
				continue
			}
			result.TotalTime = time.Since(startTime)
			return result, err
		}

		result.StatusCode = resp.StatusCode

		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			if attempt < policy.MaxRetries {
				waitTime := policy.RetryInterval * time.Duration(policy.BackoffMultiplier)
				select {
				case <-ctx.Done():
					result.TotalTime = time.Since(startTime)
					return result, ctx.Err()
				case <-time.After(waitTime):
					// Continue
				}
				continue
			}
		}

		// Success
		result.Success = true
		result.TotalTime = time.Since(startTime)
		return result, nil
	}

	result.TotalTime = time.Since(startTime)
	return result, fmt.Errorf("webhook delivery failed after %d attempts", result.Attempts)
}
