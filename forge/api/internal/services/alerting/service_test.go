package alerting

import (
	"context"
	"testing"
	"time"

	"gamepanel/forge/internal/store"
)

func TestThresholdSeverity(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		warn  float64
		crit  float64
		want  store.AlertSeverity
	}{
		{"normal usage", 50, 80, 95, store.AlertSeverityOK},
		{"warning level", 85, 80, 95, store.AlertSeverityWarning},
		{"critical level", 96, 80, 95, store.AlertSeverityCritical},
		{"exactly critical", 95, 80, 95, store.AlertSeverityCritical},
		{"exactly warning", 80, 80, 95, store.AlertSeverityWarning},
		{"zero value", 0, 80, 95, store.AlertSeverityOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := severityFromFloat(tt.value, tt.warn, tt.crit)
			if got != tt.want {
				t.Errorf("severityFromFloat(%v, %v, %v) = %v, want %v", tt.value, tt.warn, tt.crit, got, tt.want)
			}
		})
	}
}

type mockStore struct {
	alerts       []store.Alert
	routes       []store.NotificationRoute
	alertsByKey  map[string]store.Alert
}

func newMockStore() *mockStore {
	return &mockStore{
		alertsByKey: make(map[string]store.Alert),
	}
}

func (m *mockStore) CreateAlert(ctx context.Context, req store.CreateAlertRequest) (store.Alert, error) {
	alert := store.Alert{
		ID:             "alert-" + req.SuppressionKey,
		NodeID:         req.NodeID,
		ServerID:       req.ServerID,
		AlertType:      req.AlertType,
		Severity:       req.Severity,
		Title:          req.Title,
		Message:        req.Message,
		Source:         req.Source,
		SuppressionKey: req.SuppressionKey,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	m.alerts = append(m.alerts, alert)
	m.alertsByKey[req.SuppressionKey] = alert
	return alert, nil
}

func (m *mockStore) GetAlert(ctx context.Context, id string) (store.Alert, error) {
	for _, a := range m.alerts {
		if a.ID == id {
			return a, nil
		}
	}
	return store.Alert{}, nil
}

func (m *mockStore) ListAlerts(ctx context.Context, filter store.AlertFilter) ([]store.Alert, error) {
	return m.alerts, nil
}

func (m *mockStore) FindAlertBySuppressionKey(ctx context.Context, key string) (*store.Alert, error) {
	if a, ok := m.alertsByKey[key]; ok {
		return &a, nil
	}
	return nil, nil
}

func (m *mockStore) AcknowledgeAlert(ctx context.Context, id, acknowledgedBy string) error {
	for i := range m.alerts {
		if m.alerts[i].ID == id {
			m.alerts[i].Acknowledged = true
			m.alerts[i].AcknowledgedBy = acknowledgedBy
			now := time.Now()
			m.alerts[i].AcknowledgedAt = &now
			break
		}
	}
	return nil
}

func (m *mockStore) ResolveAlert(ctx context.Context, id string) error {
	for i := range m.alerts {
		if m.alerts[i].ID == id {
			m.alerts[i].Severity = store.AlertSeverityOK
			now := time.Now()
			m.alerts[i].ResolvedAt = &now
			break
		}
	}
	return nil
}

func (m *mockStore) ListNotificationRoutes(ctx context.Context, tenantID string) ([]store.NotificationRoute, error) {
	return m.routes, nil
}

func TestCreateAlert(t *testing.T) {
	ms := newMockStore()
	svc := New(nil, DefaultThresholds, nil)
	svc.store = ms

	ctx := context.Background()
	err := svc.evaluateAndAlert(ctx, "node-1", "", "cpu_high", store.AlertSeverityWarning, "High CPU", "CPU at 85%", map[string]any{"value": 85.0})
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(ms.alerts))
	}
	if ms.alerts[0].Severity != store.AlertSeverityWarning {
		t.Fatalf("expected warning severity, got %v", ms.alerts[0].Severity)
	}
}

func TestDuplicateSuppression(t *testing.T) {
	ms := newMockStore()
	svc := New(nil, DefaultThresholds, nil)
	svc.store = ms

	ctx := context.Background()
	err := svc.evaluateAndAlert(ctx, "node-1", "", "cpu_high", store.AlertSeverityWarning, "High CPU", "CPU at 85%", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = svc.evaluateAndAlert(ctx, "node-1", "", "cpu_high", store.AlertSeverityWarning, "High CPU", "CPU at 86%", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.alerts) != 1 {
		t.Fatalf("expected 1 alert (dedup), got %d", len(ms.alerts))
	}
}

func TestAlertEscalation(t *testing.T) {
	ms := newMockStore()
	svc := New(nil, DefaultThresholds, nil)
	svc.store = ms

	ctx := context.Background()
	err := svc.evaluateAndAlert(ctx, "node-1", "", "cpu_high", store.AlertSeverityWarning, "High CPU", "CPU at 85%", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = svc.evaluateAndAlert(ctx, "node-1", "", "cpu_high", store.AlertSeverityCritical, "Critical CPU", "CPU at 96%", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.alerts) != 2 {
		t.Fatalf("expected 2 alerts (escalated), got %d", len(ms.alerts))
	}
	last := ms.alerts[1]
	if last.Severity != store.AlertSeverityCritical {
		t.Fatalf("expected critical severity, got %v", last.Severity)
	}
}

func TestAlertRecovery(t *testing.T) {
	ms := newMockStore()
	svc := New(nil, DefaultThresholds, nil)
	svc.store = ms

	ctx := context.Background()
	err := svc.evaluateAndAlert(ctx, "node-1", "", "cpu_high", store.AlertSeverityWarning, "High CPU", "CPU at 85%", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = svc.evaluateAndAlert(ctx, "node-1", "", "cpu_high", store.AlertSeverityOK, "CPU Normal", "CPU at 50%", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.alerts) != 1 {
		t.Fatalf("expected 1 alert (recovered), got %d", len(ms.alerts))
	}
	if ms.alerts[0].Severity != store.AlertSeverityOK {
		t.Fatalf("expected ok severity after recovery, got %v", ms.alerts[0].Severity)
	}
}

func TestAcknowledgeAlert(t *testing.T) {
	ms := newMockStore()
	svc := New(nil, DefaultThresholds, nil)
	svc.store = ms

	ctx := context.Background()
	err := svc.evaluateAndAlert(ctx, "node-1", "", "cpu_high", store.AlertSeverityWarning, "High CPU", "CPU at 85%", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.AcknowledgeAlert(ctx, ms.alerts[0].ID, "admin@test.com"); err != nil {
		t.Fatal(err)
	}
	alert, err := svc.GetAlert(ctx, ms.alerts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !alert.Acknowledged {
		t.Fatal("expected alert to be acknowledged")
	}
	if alert.AcknowledgedBy != "admin@test.com" {
		t.Fatalf("expected acknowledged_by 'admin@test.com', got %q", alert.AcknowledgedBy)
	}
}

func TestResolveAlert(t *testing.T) {
	ms := newMockStore()
	svc := New(nil, DefaultThresholds, nil)
	svc.store = ms

	ctx := context.Background()
	err := svc.evaluateAndAlert(ctx, "node-1", "", "cpu_high", store.AlertSeverityWarning, "High CPU", "CPU at 85%", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.ResolveAlert(ctx, ms.alerts[0].ID); err != nil {
		t.Fatal(err)
	}
	alert, err := svc.GetAlert(ctx, ms.alerts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if alert.Severity != store.AlertSeverityOK {
		t.Fatalf("expected ok severity after resolve, got %v", alert.Severity)
	}
}

func TestStaleHeartbeat(t *testing.T) {
	ms := newMockStore()
	svc := New(nil, DefaultThresholds, nil)
	svc.store = ms

	ctx := context.Background()
	now := time.Now()

	err := svc.CheckStaleHeartbeat(ctx, "node-1", &now, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.alerts) != 0 {
		t.Fatal("expected no alert for fresh heartbeat")
	}

	past := time.Now().Add(-10 * time.Minute)
	err = svc.CheckStaleHeartbeat(ctx, "node-1", &past, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.alerts) == 0 {
		t.Fatal("expected alert for stale heartbeat")
	}
}

func TestNodeThresholds(t *testing.T) {
	ms := newMockStore()
	svc := New(nil, DefaultThresholds, nil)
	svc.store = ms

	ctx := context.Background()
	metrics := store.NodeMetric{
		NodeID:        "node-1",
		CPUPercent:    50.0,
		MemoryPercent: 85.0,
		DiskPercent:   90.0,
	}
	err := svc.CheckNodeThresholds(ctx, metrics)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.alerts) != 2 {
		t.Fatalf("expected 2 alerts (memory + disk), got %d: %+v", len(ms.alerts), ms.alerts)
	}
	hasMemory := false
	hasDisk := false
	for _, a := range ms.alerts {
		if a.AlertType == "memory_high" {
			hasMemory = true
			if a.Severity != store.AlertSeverityWarning {
				t.Fatalf("expected memory warning, got %v", a.Severity)
			}
		}
		if a.AlertType == "disk_high" {
			hasDisk = true
			if a.Severity != store.AlertSeverityCritical {
				t.Fatalf("expected disk critical, got %v", a.Severity)
			}
		}
	}
	if !hasMemory {
		t.Fatal("missing memory_high alert")
	}
	if !hasDisk {
		t.Fatal("missing disk_high alert")
	}
}

func TestSeverityFromFloat(t *testing.T) {
	tests := []struct {
		val  float64
		warn float64
		crit float64
		want store.AlertSeverity
	}{
		{10, 80, 95, store.AlertSeverityOK},
		{80, 80, 95, store.AlertSeverityWarning},
		{90, 80, 95, store.AlertSeverityWarning},
		{95, 80, 95, store.AlertSeverityCritical},
		{99, 80, 95, store.AlertSeverityCritical},
	}
	for _, tt := range tests {
		got := severityFromFloat(tt.val, tt.warn, tt.crit)
		if got != tt.want {
			t.Errorf("severityFromFloat(%v, %v, %v) = %v, want %v", tt.val, tt.warn, tt.crit, got, tt.want)
		}
	}
}

func TestContainsEventType(t *testing.T) {
	if !containsEventType([]string{"cpu_high", "memory_high"}, "cpu_high") {
		t.Fatal("expected to match cpu_high")
	}
	if containsEventType([]string{"cpu_high"}, "memory_high") {
		t.Fatal("expected not to match memory_high")
	}
	if !containsEventType([]string{"*"}, "anything") {
		t.Fatal("expected wildcard to match everything")
	}
	if !containsEventType(nil, "anything") {
		t.Fatal("expected nil to match everything")
	}
}
