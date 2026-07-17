package cron

import (
	"context"
	"testing"
	"time"
)

func TestSFTPCronDeduplication(t *testing.T) {
	client := &mockPanelClient{}
	now := time.Now()
	manager := &mockServerManager{
		client: client,
		events: []ActivityEvent{
			{ID: 1, User: "user1", Server: "srv1", Event: "server:sftp.login", IP: "1.2.3.4", Timestamp: now, Metadata: map[string]interface{}{"files": []interface{}{"a.txt"}}},
			{ID: 2, User: "user1", Server: "srv1", Event: "server:sftp.login", IP: "1.2.3.4", Timestamp: now.Add(10 * time.Second), Metadata: map[string]interface{}{"files": []interface{}{"b.txt"}}},
			{ID: 3, User: "user2", Server: "srv1", Event: "server:sftp.login", IP: "1.2.3.4", Timestamp: now, Metadata: map[string]interface{}{"files": []interface{}{"c.txt"}}},
		},
	}
	sc := NewSFTPCron(manager, 100)
	if err := sc.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.activities) != 2 {
		t.Fatalf("expected 2 deduplicated activities, got %d", len(client.activities))
	}
	var found bool
	for _, a := range client.activities {
		if a.User == "user1" {
			found = true
			files, ok := a.Metadata["files"].([]interface{})
			if !ok {
				t.Fatal("expected files metadata")
			}
			if len(files) != 2 {
				t.Fatalf("expected 2 merged files, got %d", len(files))
			}
		}
	}
	if !found {
		t.Fatal("user1 activity not found")
	}
}

func TestSFTPCronFiltersNonSFTP(t *testing.T) {
	client := &mockPanelClient{}
	manager := &mockServerManager{
		client: client,
		events: []ActivityEvent{
			{ID: 1, Event: "server:power.start", IP: "1.2.3.4", Timestamp: time.Now()},
			{ID: 2, Event: "server:sftp.login", IP: "1.2.3.4", Timestamp: time.Now(), Metadata: map[string]interface{}{}},
		},
	}
	sc := NewSFTPCron(manager, 100)
	if err := sc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.activities) != 1 {
		t.Fatalf("expected 1 SFTP activity, got %d", len(client.activities))
	}
	if client.activities[0].Event != "server:sftp.login" {
		t.Fatalf("unexpected event: %s", client.activities[0].Event)
	}
}

func TestSFTPCronEmptyEvents(t *testing.T) {
	client := &mockPanelClient{}
	manager := &mockServerManager{client: client, events: nil}
	sc := NewSFTPCron(manager, 100)
	if err := sc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.activities) != 0 {
		t.Fatal("expected no activities")
	}
}

func TestSFTPCronDeleteInChunks(t *testing.T) {
	client := &mockPanelClient{}
	manager := &mockServerManager{client: client}
	sc := NewSFTPCron(manager, 100)
	ids := make([]int, 100)
	for i := range ids {
		ids[i] = i + 1
	}
	if err := sc.deleteInChunks(ids); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.deleted) != 100 {
		t.Fatalf("expected 100 deleted, got %d", len(manager.deleted))
	}
}

func TestSFTPCronConcurrentGuard(t *testing.T) {
	client := &mockPanelClient{}
	manager := &mockServerManager{
		client: client,
		events: []ActivityEvent{
			{ID: 1, Event: "server:sftp.login", IP: "1.2.3.4", Timestamp: time.Now(), Metadata: map[string]interface{}{}},
		},
	}
	sc := NewSFTPCron(manager, 100)
	sc.mu.Lock()
	sc.running = true
	sc.mu.Unlock()
	if err := sc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.activities) != 0 {
		t.Fatal("concurrent run should have been skipped")
	}
}

func TestSFTPCronInvalidIP(t *testing.T) {
	client := &mockPanelClient{}
	manager := &mockServerManager{
		client: client,
		events: []ActivityEvent{
			{ID: 1, Event: "server:sftp.login", IP: "invalid", Timestamp: time.Now(), Metadata: map[string]interface{}{}},
		},
	}
	sc := NewSFTPCron(manager, 100)
	if err := sc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.activities[0].IP != "" {
		t.Fatalf("expected empty IP, got %q", client.activities[0].IP)
	}
}

func TestSFTPCronDefaultBatch(t *testing.T) {
	sc := NewSFTPCron(nil, 0)
	if sc.maxBatch != 100 {
		t.Fatalf("expected default batch 100, got %d", sc.maxBatch)
	}
}

func TestSFTPCronDedupByMinute(t *testing.T) {
	client := &mockPanelClient{}
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	manager := &mockServerManager{
		client: client,
		events: []ActivityEvent{
			{ID: 1, User: "u1", Server: "s1", Event: "server:sftp.login", IP: "1.1.1.1", Timestamp: base.Add(5 * time.Second), Metadata: map[string]interface{}{"files": []interface{}{"x.txt"}}},
			{ID: 2, User: "u1", Server: "s1", Event: "server:sftp.login", IP: "1.1.1.1", Timestamp: base.Add(30 * time.Second), Metadata: map[string]interface{}{"files": []interface{}{"y.txt"}}},
			{ID: 3, User: "u1", Server: "s1", Event: "server:sftp.login", IP: "1.1.1.1", Timestamp: base.Add(61 * time.Second), Metadata: map[string]interface{}{"files": []interface{}{"z.txt"}}},
		},
	}
	sc := NewSFTPCron(manager, 100)
	if err := sc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.activities) != 2 {
		t.Fatalf("expected 2 deduplicated activities (minute boundary), got %d", len(client.activities))
	}
}

func TestSFTPCronNilClient(t *testing.T) {
	manager := &mockServerManager{
		client: nil,
		events: []ActivityEvent{
			{ID: 1, Event: "server:sftp.login", IP: "1.2.3.4", Timestamp: time.Now(), Metadata: map[string]interface{}{}},
		},
	}
	sc := NewSFTPCron(manager, 100)
	if err := sc.Run(context.Background()); err != nil {
		t.Fatalf("expected nil error with nil client, got: %v", err)
	}
}

func TestSFTPCronBatchLimit(t *testing.T) {
	client := &mockPanelClient{}
	events := make([]ActivityEvent, 200)
	for i := range events {
		events[i] = ActivityEvent{
			ID:        i + 1,
			Event:     "server:sftp.login",
			IP:        "1.2.3.4",
			Timestamp: time.Now(),
			Metadata:  map[string]interface{}{},
		}
	}
	manager := &mockServerManager{client: client, events: events}
	sc := NewSFTPCron(manager, 50)
	if err := sc.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.deleted) > 50 {
		t.Fatalf("expected at most 50 events processed, got %d", len(manager.deleted))
	}
}
