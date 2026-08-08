package cron

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gamepanel/beacon/internal/remote"
)

type mockPanelClient struct {
	mu         sync.Mutex
	activities []remote.Activity
	sendErr    error
}

func (m *mockPanelClient) GetServerConfiguration(ctx context.Context, uuid string) (remote.ServerConfigurationResponse, error) {
	return remote.ServerConfigurationResponse{}, nil
}
func (m *mockPanelClient) GetServers(ctx context.Context, perPage int) ([]remote.RawServerData, error) {
	return nil, nil
}
func (m *mockPanelClient) ResetServersState(ctx context.Context) error { return nil }
func (m *mockPanelClient) SendActivityLogs(ctx context.Context, activity []remote.Activity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.activities = append(m.activities, activity...)
	return nil
}
func (m *mockPanelClient) SendServerStats(ctx context.Context, serverID string, stats remote.ServerStats) error {
	return nil
}
func (m *mockPanelClient) SendNodeHeartbeat(ctx context.Context, nodeID string, heartbeat remote.NodeHeartbeat) error {
	return nil
}
func (m *mockPanelClient) CreatePlacementReservation(ctx context.Context, req remote.PlacementReservationRequest) (remote.PlacementReservation, error) {
	return remote.PlacementReservation{}, nil
}
func (m *mockPanelClient) ConfirmPlacementReservation(ctx context.Context, reservationID string) error {
	return nil
}
func (m *mockPanelClient) CancelPlacementReservation(ctx context.Context, reservationID string) error {
	return nil
}
func (m *mockPanelClient) TriggerServerBackup(ctx context.Context, serverID string) error { return nil }
func (m *mockPanelClient) ReportEvacuationProgress(ctx context.Context, evacuationID string, progress remote.EvacuationProgress) error {
	return nil
}
func (m *mockPanelClient) SetInstallationStatus(ctx context.Context, serverID string, successful bool) error {
	return nil
}
func (m *mockPanelClient) SendCrashEvent(ctx context.Context, serverID string, exitCode int, oomKilled bool, autoRestart bool) error {
	return nil
}
func (m *mockPanelClient) SendBackupStatus(ctx context.Context, serverID string, req remote.BackupStatusRequest) error {
	return nil
}
func (m *mockPanelClient) SendRestoreStatus(ctx context.Context, serverID string, req remote.RestoreStatusRequest) error {
	return nil
}
func (m *mockPanelClient) SendCapabilityReport(ctx context.Context, report interface{}) error {
	return nil
}

type mockServerManager struct {
	mu        sync.Mutex
	client    remote.Client
	events    []ActivityEvent
	deleted   []int
	fetchErr  error
	deleteErr error
}

func (m *mockServerManager) GetPanelClient() remote.Client {
	return m.client
}

func (m *mockServerManager) GetActivityEvents(limit int) ([]ActivityEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	if len(m.events) > limit {
		return m.events[:limit], nil
	}
	return m.events, nil
}

func (m *mockServerManager) DeleteActivityEvents(ids []int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deleted = append(m.deleted, ids...)
	return nil
}

func TestActivityCronRun(t *testing.T) {
	client := &mockPanelClient{}
	manager := &mockServerManager{
		client: client,
		events: []ActivityEvent{
			{ID: 1, User: "user1", Server: "srv1", Event: "server:power.start", IP: "1.2.3.4", Timestamp: time.Now(), Metadata: map[string]interface{}{}},
			{ID: 2, User: "user2", Server: "srv2", Event: "server:power.stop", IP: "5.6.7.8", Timestamp: time.Now(), Metadata: map[string]interface{}{}},
		},
	}
	ac := NewActivityCron(manager, 100)
	if err := ac.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	client.mu.Lock()
	if len(client.activities) != 2 {
		t.Fatalf("expected 2 activities sent, got %d", len(client.activities))
	}
	client.mu.Unlock()
	manager.mu.Lock()
	if len(manager.deleted) != 2 {
		t.Fatalf("expected 2 events deleted, got %d", len(manager.deleted))
	}
	manager.mu.Unlock()
}

func TestActivityCronFiltersSFTP(t *testing.T) {
	client := &mockPanelClient{}
	manager := &mockServerManager{
		client: client,
		events: []ActivityEvent{
			{ID: 1, Event: "server:sftp.login", IP: "1.2.3.4", Timestamp: time.Now()},
			{ID: 2, Event: "server:power.start", IP: "1.2.3.4", Timestamp: time.Now()},
		},
	}
	ac := NewActivityCron(manager, 100)
	if err := ac.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	if len(client.activities) != 1 {
		t.Fatalf("expected 1 non-SFTP activity, got %d", len(client.activities))
	}
	if client.activities[0].Event != "server:power.start" {
		t.Fatalf("unexpected event: %s", client.activities[0].Event)
	}
	client.mu.Unlock()
}

func TestActivityCronInvalidIP(t *testing.T) {
	client := &mockPanelClient{}
	manager := &mockServerManager{
		client: client,
		events: []ActivityEvent{
			{ID: 1, Event: "server:power.start", IP: "not-an-ip", Timestamp: time.Now()},
		},
	}
	ac := NewActivityCron(manager, 100)
	if err := ac.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	if len(client.activities) != 1 {
		t.Fatal("expected 1 activity")
	}
	if client.activities[0].IP != "" {
		t.Fatalf("expected empty IP for invalid address, got %q", client.activities[0].IP)
	}
	client.mu.Unlock()
}

func TestActivityCronEmptyEvents(t *testing.T) {
	client := &mockPanelClient{}
	manager := &mockServerManager{client: client, events: nil}
	ac := NewActivityCron(manager, 100)
	if err := ac.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	if len(client.activities) != 0 {
		t.Fatal("expected no activities sent")
	}
	client.mu.Unlock()
}

func TestActivityCronFetchError(t *testing.T) {
	manager := &mockServerManager{fetchErr: errors.New("db error")}
	ac := NewActivityCron(manager, 100)
	if err := ac.Run(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestActivityCronSendError(t *testing.T) {
	client := &mockPanelClient{sendErr: errors.New("send failed")}
	manager := &mockServerManager{
		client: client,
		events: []ActivityEvent{
			{ID: 1, Event: "server:power.start", IP: "1.2.3.4", Timestamp: time.Now()},
		},
	}
	ac := NewActivityCron(manager, 100)
	if err := ac.Run(context.Background()); err == nil {
		t.Fatal("expected error on send failure")
	}
}

func TestActivityCronConcurrentGuard(t *testing.T) {
	client := &mockPanelClient{}
	manager := &mockServerManager{
		client: client,
		events: []ActivityEvent{
			{ID: 1, Event: "server:power.start", IP: "1.2.3.4", Timestamp: time.Now()},
		},
	}
	ac := NewActivityCron(manager, 100)
	ac.mu.Lock()
	ac.running = true
	ac.mu.Unlock()
	if err := ac.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	if len(client.activities) != 0 {
		t.Fatal("concurrent run should have been skipped")
	}
	client.mu.Unlock()
}

func TestActivityCronDefaultBatch(t *testing.T) {
	ac := NewActivityCron(nil, 0)
	if ac.maxBatch != 100 {
		t.Fatalf("expected default batch 100, got %d", ac.maxBatch)
	}
}

func TestActivityCronNilClient(t *testing.T) {
	manager := &mockServerManager{
		client: nil,
		events: []ActivityEvent{
			{ID: 1, Event: "server:power.start", IP: "1.2.3.4", Timestamp: time.Now()},
		},
	}
	ac := NewActivityCron(manager, 100)
	if err := ac.Run(context.Background()); err != nil {
		t.Fatalf("expected nil error with nil client, got: %v", err)
	}
}
