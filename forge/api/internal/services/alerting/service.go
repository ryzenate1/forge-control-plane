package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	notificationsvc "gamepanel/forge/internal/services/notifications"
	"gamepanel/forge/internal/store"
)

type Store interface {
	CreateAlert(ctx context.Context, req store.CreateAlertRequest) (store.Alert, error)
	GetAlert(ctx context.Context, id string) (store.Alert, error)
	ListAlerts(ctx context.Context, filter store.AlertFilter) ([]store.Alert, error)
	FindAlertBySuppressionKey(ctx context.Context, key string) (*store.Alert, error)
	AcknowledgeAlert(ctx context.Context, id, acknowledgedBy string) error
	ResolveAlert(ctx context.Context, id string) error
	ListNotificationRoutes(ctx context.Context, tenantID string) ([]store.NotificationRoute, error)
}

type ThresholdConfig struct {
	CPUWarning     float64
	CPUCritical    float64
	MemoryWarning  float64
	MemoryCritical float64
	DiskWarning    float64
	DiskCritical   float64
}

var DefaultThresholds = ThresholdConfig{
	CPUWarning:     80.0,
	CPUCritical:    95.0,
	MemoryWarning:  80.0,
	MemoryCritical: 95.0,
	DiskWarning:    85.0,
	DiskCritical:   90.0,
}

type Service struct {
	store         Store
	thresholds    ThresholdConfig
	notifier      *Notifier
	logger        *slog.Logger
	mu            sync.RWMutex
	routes        []store.NotificationRoute
	dispatchSlots chan struct{}
}

func New(s Store, thresholds ThresholdConfig, logger *slog.Logger) *Service {
	svc := &Service{
		store:         s,
		thresholds:    thresholds,
		notifier:      NewNotifier(&http.Client{Timeout: 10 * time.Second}, logger),
		logger:        logger,
		dispatchSlots: make(chan struct{}, 16),
	}
	return svc
}

func (svc *Service) RefreshRoutes(ctx context.Context) error {
	routes, err := svc.store.ListNotificationRoutes(ctx, "")
	if err != nil {
		return err
	}
	svc.mu.Lock()
	svc.routes = routes
	svc.mu.Unlock()
	return nil
}

func (svc *Service) getRoutes() []store.NotificationRoute {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	routes := make([]store.NotificationRoute, len(svc.routes))
	copy(routes, svc.routes)
	return routes
}

func severityFromFloat(val float64, warn, crit float64) store.AlertSeverity {
	if val >= crit {
		return store.AlertSeverityCritical
	}
	if val >= warn {
		return store.AlertSeverityWarning
	}
	return store.AlertSeverityOK
}

func (svc *Service) CheckNodeThresholds(ctx context.Context, metrics store.NodeMetric) error {
	cpuSev := severityFromFloat(metrics.CPUPercent, svc.thresholds.CPUWarning, svc.thresholds.CPUCritical)
	memSev := severityFromFloat(metrics.MemoryPercent, svc.thresholds.MemoryWarning, svc.thresholds.MemoryCritical)
	diskSev := severityFromFloat(metrics.DiskPercent, svc.thresholds.DiskWarning, svc.thresholds.DiskCritical)

	for _, check := range []struct {
		alertType string
		severity  store.AlertSeverity
		value     float64
		message   string
		title     string
	}{
		{"cpu_high", cpuSev, metrics.CPUPercent,
			fmt.Sprintf("CPU at %.1f%% (thresholds: warning=%.0f%%, critical=%.0f%%)", metrics.CPUPercent, svc.thresholds.CPUWarning, svc.thresholds.CPUCritical),
			"High CPU Usage"},
		{"memory_high", memSev, metrics.MemoryPercent,
			fmt.Sprintf("Memory at %.1f%% (%d/%d MB used) (thresholds: warning=%.0f%%, critical=%.0f%%)",
				metrics.MemoryPercent, metrics.MemoryUsedMB, metrics.MemoryTotalMB, svc.thresholds.MemoryWarning, svc.thresholds.MemoryCritical),
			"High Memory Usage"},
		{"disk_high", diskSev, metrics.DiskPercent,
			fmt.Sprintf("Disk at %.1f%% (%d/%d MB used) (thresholds: warning=%.0f%%, critical=%.0f%%)",
				metrics.DiskPercent, metrics.DiskUsedMB, metrics.DiskTotalMB, svc.thresholds.DiskWarning, svc.thresholds.DiskCritical),
			"High Disk Usage"},
	} {
		if err := svc.evaluateAndAlert(ctx, metrics.NodeID, "", check.alertType, check.severity, check.title, check.message, map[string]any{
			"node_id":    metrics.NodeID,
			"value":      math.Round(check.value*10) / 10,
			"threshold":  check.severity,
			"observedAt": metrics.ObservedAt,
		}); err != nil {
			svc.logger.Error("failed to evaluate alert", "alertType", check.alertType, "error", err)
		}
	}
	return nil
}

func (svc *Service) CheckStaleHeartbeat(ctx context.Context, nodeID string, lastSeenAt *time.Time, maxAge time.Duration) error {
	if lastSeenAt == nil {
		return svc.evaluateAndAlert(ctx, nodeID, "", "stale_heartbeat", store.AlertSeverityCritical,
			"Node Heartbeat Missing",
			fmt.Sprintf("Node %s has never reported a heartbeat", nodeID),
			map[string]any{"node_id": nodeID, "lastSeenAt": nil})
	}
	age := time.Since(*lastSeenAt)
	if age > maxAge {
		minutes := int(age.Minutes())
		sev := store.AlertSeverityWarning
		if minutes > 10 {
			sev = store.AlertSeverityCritical
		}
		return svc.evaluateAndAlert(ctx, nodeID, "", "stale_heartbeat", sev,
			"Stale Node Heartbeat",
			fmt.Sprintf("Node %s last seen %d minutes ago (max allowed: %d minutes)", nodeID, minutes, int(maxAge.Minutes())),
			map[string]any{"node_id": nodeID, "age_minutes": minutes, "lastSeenAt": lastSeenAt})
	}

	existing, err := svc.store.FindAlertBySuppressionKey(ctx, "stale_heartbeat:"+nodeID)
	if err == nil && existing != nil && existing.Severity != store.AlertSeverityOK {
		if err := svc.ResolveAlert(ctx, existing.ID); err != nil {
			svc.logger.Error("failed to resolve stale heartbeat alert", "alertID", existing.ID, "error", err)
		}
	}
	return nil
}

func (svc *Service) evaluateAndAlert(ctx context.Context, nodeID, serverID, alertType string, severity store.AlertSeverity, title, message string, details map[string]any) error {
	if severity == store.AlertSeverityOK {
		return svc.resolveIfActive(ctx, alertType, nodeID, serverID)
	}

	suppressionKey := alertType + ":" + nodeID
	if serverID != "" {
		suppressionKey = alertType + ":" + serverID
	}

	existing, err := svc.store.FindAlertBySuppressionKey(ctx, suppressionKey)
	if err != nil {
		return err
	}

	if existing != nil {
		if existing.Severity == severity && existing.Acknowledged {
			return nil
		}
		if existing.Severity == severity {
			return nil
		}
	}

	alert, err := svc.store.CreateAlert(ctx, store.CreateAlertRequest{
		NodeID:         nodeID,
		ServerID:       serverID,
		AlertType:      alertType,
		Severity:       severity,
		Title:          title,
		Message:        message,
		Details:        details,
		Source:         "monitor",
		SuppressionKey: suppressionKey,
	})
	if err != nil {
		return err
	}

	dispatchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	select {
	case svc.dispatchSlots <- struct{}{}:
		go func() {
			defer cancel()
			defer func() { <-svc.dispatchSlots }()
			svc.dispatchNotifications(dispatchCtx, alert)
		}()
	case <-dispatchCtx.Done():
		cancel()
		svc.logger.Error("notification dispatch capacity exhausted", "alert", alert.ID)
	}
	return nil
}

func (svc *Service) resolveIfActive(ctx context.Context, alertType, nodeID, serverID string) error {
	suppressionKey := alertType + ":" + nodeID
	if serverID != "" {
		suppressionKey = alertType + ":" + serverID
	}
	existing, err := svc.store.FindAlertBySuppressionKey(ctx, suppressionKey)
	if err != nil || existing == nil {
		return err
	}
	if existing.Severity == store.AlertSeverityOK {
		return nil
	}
	return svc.ResolveAlert(ctx, existing.ID)
}

func (svc *Service) ResolveAlert(ctx context.Context, alertID string) error {
	return svc.store.ResolveAlert(ctx, alertID)
}

func (svc *Service) AcknowledgeAlert(ctx context.Context, alertID, acknowledgedBy string) error {
	return svc.store.AcknowledgeAlert(ctx, alertID, acknowledgedBy)
}

func (svc *Service) ListAlerts(ctx context.Context, filter store.AlertFilter) ([]store.Alert, error) {
	return svc.store.ListAlerts(ctx, filter)
}

func (svc *Service) GetAlert(ctx context.Context, id string) (store.Alert, error) {
	return svc.store.GetAlert(ctx, id)
}

func (svc *Service) dispatchNotifications(ctx context.Context, alert store.Alert) {
	routes := svc.getRoutes()
	severityScore := map[store.AlertSeverity]int{
		store.AlertSeverityOK:       0,
		store.AlertSeverityWarning:  1,
		store.AlertSeverityCritical: 2,
	}
	alertScore := severityScore[alert.Severity]

	for _, route := range routes {
		if !route.Enabled {
			continue
		}
		routeScore := severityScore[route.MinSeverity]
		if alertScore < routeScore {
			continue
		}
		if len(route.EventTypes) > 0 && !containsEventType(route.EventTypes, alert.AlertType) {
			continue
		}

		if err := svc.notifier.Send(ctx, route, alert); err != nil {
			svc.logger.Error("notification dispatch failed",
				"route", route.Name, "channel", route.ChannelType, "alert", alert.ID, "error", err)
		}
	}
}

func containsEventType(types []string, event string) bool {
	if len(types) == 0 {
		return true
	}
	for _, t := range types {
		if t == "*" || t == event {
			return true
		}
	}
	return false
}

type Notifier struct {
	client *http.Client
	logger *slog.Logger
}

func NewNotifier(client *http.Client, logger *slog.Logger) *Notifier {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Notifier{client: client, logger: logger}
}

func (n *Notifier) Send(ctx context.Context, route store.NotificationRoute, alert store.Alert) error {
	switch route.ChannelType {
	case "slack":
		return n.sendSlack(ctx, route.Config, alert)
	case "discord":
		return n.sendDiscord(ctx, route.Config, alert)
	case "telegram":
		return n.sendTelegram(ctx, route.Config, alert)
	case "email":
		return n.sendEmail(ctx, route.Config, alert)
	case "webhook":
		return n.sendWebhook(ctx, route.Config, alert)
	default:
		return fmt.Errorf("unknown notification channel: %s", route.ChannelType)
	}
}

func (n *Notifier) sendSlack(ctx context.Context, config map[string]any, alert store.Alert) error {
	webhookURL, _ := config["webhook_url"].(string)
	if webhookURL == "" {
		return fmt.Errorf("slack webhook_url not configured")
	}
	color := "#ff0000"
	if alert.Severity == store.AlertSeverityWarning {
		color = "#ffa500"
	} else if alert.Severity == store.AlertSeverityOK {
		color = "#00ff00"
	}
	payload := map[string]any{
		"attachments": []any{map[string]any{
			"color":  color,
			"title":  alert.Title,
			"text":   alert.Message,
			"fields": flatFields(alert.Details),
			"ts":     alert.CreatedAt.Unix(),
		}},
	}
	return n.postJSON(ctx, webhookURL, payload)
}

func (n *Notifier) sendDiscord(ctx context.Context, config map[string]any, alert store.Alert) error {
	webhookURL, _ := config["webhook_url"].(string)
	if webhookURL == "" {
		return fmt.Errorf("discord webhook_url not configured")
	}
	color := 0xff0000
	if alert.Severity == store.AlertSeverityWarning {
		color = 0xffa500
	} else if alert.Severity == store.AlertSeverityOK {
		color = 0x00ff00
	}
	payload := map[string]any{
		"embeds": []any{map[string]any{
			"title":       alert.Title,
			"description": alert.Message,
			"color":       color,
			"fields":      flatFields(alert.Details),
			"timestamp":   alert.CreatedAt.Format(time.RFC3339),
		}},
	}
	return n.postJSON(ctx, webhookURL, payload)
}

func (n *Notifier) sendTelegram(ctx context.Context, config map[string]any, alert store.Alert) error {
	botToken, _ := config["bot_token"].(string)
	chatID, _ := config["chat_id"].(string)
	if botToken == "" || chatID == "" {
		return fmt.Errorf("telegram bot_token and chat_id required")
	}
	text := fmt.Sprintf("%s [%s]\n%s", alert.Title, strings.ToUpper(string(alert.Severity)), alert.Message)
	if len(alert.Details) > 0 {
		details, _ := json.MarshalIndent(alert.Details, "", "  ")
		text += "\n\n" + string(details)
	}
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	return n.postJSON(ctx, apiURL, payload)
}

func (n *Notifier) sendEmail(ctx context.Context, config map[string]any, alert store.Alert) error {
	recipientsRaw, _ := config["recipients"].([]any)
	recipients := make([]string, 0, len(recipientsRaw))
	for _, raw := range recipientsRaw {
		if recipient, ok := raw.(string); ok {
			recipients = append(recipients, recipient)
		}
	}
	if len(recipients) == 0 {
		return fmt.Errorf("email recipients not configured")
	}
	port := 0
	if raw, ok := config["smtp_port"].(float64); ok {
		port = int(raw)
	}
	emailer := notificationsvc.NewEmailService(notificationsvc.EmailConfig{
		SMTPHost:     stringConfig(config, "smtp_host"),
		SMTPPort:     port,
		SMTPUsername: stringConfig(config, "smtp_username"),
		SMTPPassword: stringConfig(config, "smtp_password"),
		FromAddress:  stringConfig(config, "from_address"),
		FromName:     stringConfig(config, "from_name"),
		UseTLS:       boolConfig(config, "use_tls"),
		UseSSL:       boolConfig(config, "use_ssl"),
	})
	return emailer.Send(ctx, notificationsvc.EmailMessage{
		To:      recipients,
		Subject: alert.Title,
		Body:    alert.Message,
	})
}

func (n *Notifier) sendWebhook(ctx context.Context, config map[string]any, alert store.Alert) error {
	url, _ := config["url"].(string)
	if url == "" {
		return fmt.Errorf("webhook url not configured")
	}
	headers, _ := config["headers"].(map[string]any)
	body := map[string]any{
		"event":     "alert",
		"id":        alert.ID,
		"alertType": alert.AlertType,
		"severity":  alert.Severity,
		"title":     alert.Title,
		"message":   alert.Message,
		"nodeId":    alert.NodeID,
		"serverId":  alert.ServerID,
		"details":   alert.Details,
		"timestamp": alert.CreatedAt,
	}
	return n.postJSONWithHeaders(ctx, url, body, headers)
}

func (n *Notifier) postJSON(ctx context.Context, url string, payload any) error {
	return n.postJSONWithHeaders(ctx, url, payload, nil)
}

func (n *Notifier) postJSONWithHeaders(ctx context.Context, url string, payload any, headers map[string]any) error {
	if err := validateNotificationURL(url); err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		if s, ok := v.(string); ok {
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "host", "content-length", "connection", "transfer-encoding":
				continue
			}
			req.Header.Set(k, s)
		}
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return errors.New("notification request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notification returned status %d", resp.StatusCode)
	}
	return nil
}

func stringConfig(config map[string]any, key string) string {
	value, _ := config[key].(string)
	return value
}

func boolConfig(config map[string]any, key string) bool {
	value, _ := config[key].(bool)
	return value
}

func validateNotificationURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return errors.New("notification URL must be an absolute HTTPS URL")
	}
	addresses, err := net.LookupIP(parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return errors.New("notification host does not resolve")
	}
	for _, address := range addresses {
		if address.IsPrivate() || address.IsLoopback() || address.IsUnspecified() ||
			address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() {
			return errors.New("notification URL resolves to a non-public address")
		}
	}
	return nil
}

func flatFields(details map[string]any) []any {
	fields := []any{}
	for k, v := range details {
		fields = append(fields, map[string]any{
			"name":   k,
			"value":  fmt.Sprintf("%v", v),
			"inline": true,
		})
	}
	return fields
}
