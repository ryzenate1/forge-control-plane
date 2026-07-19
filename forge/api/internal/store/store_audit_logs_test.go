package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"gamepanel/forge/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mockAuditLogRow struct {
	id           string
	userID       string
	action       string
	resourceType string
	resourceID   string
	detailsJSON  []byte
	ipAddress    string
	userAgent    string
	createdAt    time.Time
}

type mockAuditLogRows struct {
	rows    []mockAuditLogRow
	pos     int
	scanErr error
	rowsErr error
}

func (m *mockAuditLogRows) Next() bool {
	if m.pos >= len(m.rows) {
		return false
	}
	m.pos++
	return true
}

func (m *mockAuditLogRows) Scan(dest ...any) error {
	if m.scanErr != nil {
		return m.scanErr
	}
	if m.pos <= 0 || m.pos > len(m.rows) {
		return errors.New("scan: no row")
	}
	r := m.rows[m.pos-1]
	for i, d := range dest {
		switch i {
		case 0:
			*d.(*string) = r.id
		case 1:
			*d.(*string) = r.userID
		case 2:
			*d.(*string) = r.action
		case 3:
			*d.(*string) = r.resourceType
		case 4:
			*d.(*string) = r.resourceID
		case 5:
			*d.(*[]byte) = r.detailsJSON
		case 6:
			*d.(*string) = r.ipAddress
		case 7:
			*d.(*string) = r.userAgent
		case 8:
			*d.(*time.Time) = r.createdAt
		}
	}
	return nil
}

func (m *mockAuditLogRows) Close() {}

func (m *mockAuditLogRows) Err() error { return m.rowsErr }

func TestAuditLogScan(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		logs, err := scanAuditLogs(&mockAuditLogRows{})
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) != 0 {
			t.Fatalf("expected 0 logs, got %d", len(logs))
		}
	})

	t.Run("single row without details", func(t *testing.T) {
		now := time.Now().Truncate(time.Microsecond)
		rows := &mockAuditLogRows{
			rows: []mockAuditLogRow{{
				id: uuid.NewString(), userID: uuid.NewString(),
				action: "server.create", resourceType: "server", resourceID: uuid.NewString(),
				ipAddress: "10.0.0.1", userAgent: "test-agent",
				createdAt: now,
			}},
		}
		logs, err := scanAuditLogs(rows)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) != 1 {
			t.Fatalf("expected 1 log, got %d", len(logs))
		}
		if logs[0].ID != rows.rows[0].id {
			t.Fatalf("ID = %q, want %q", logs[0].ID, rows.rows[0].id)
		}
		if logs[0].UserID != rows.rows[0].userID {
			t.Fatalf("UserID = %q, want %q", logs[0].UserID, rows.rows[0].userID)
		}
		if logs[0].Action != "server.create" {
			t.Fatalf("Action = %q, want %q", logs[0].Action, "server.create")
		}
		if logs[0].ResourceType != "server" {
			t.Fatalf("ResourceType = %q, want %q", logs[0].ResourceType, "server")
		}
		if logs[0].ResourceID != rows.rows[0].resourceID {
			t.Fatalf("ResourceID = %q, want %q", logs[0].ResourceID, rows.rows[0].resourceID)
		}
		if logs[0].IPAddress != "10.0.0.1" {
			t.Fatalf("IPAddress = %q, want %q", logs[0].IPAddress, "10.0.0.1")
		}
		if logs[0].UserAgent != "test-agent" {
			t.Fatalf("UserAgent = %q, want %q", logs[0].UserAgent, "test-agent")
		}
		if !logs[0].CreatedAt.Equal(now) {
			t.Fatalf("CreatedAt = %v, want %v", logs[0].CreatedAt, now)
		}
	})

	t.Run("row with details", func(t *testing.T) {
		details := models.JSONMap{"key": "value", "count": float64(42)}
		detailsJSON, _ := json.Marshal(details)
		rows := &mockAuditLogRows{
			rows: []mockAuditLogRow{{
				id: uuid.NewString(), userID: uuid.NewString(),
				action: "user.update", resourceType: "user", resourceID: uuid.NewString(),
				detailsJSON: detailsJSON,
				createdAt:   time.Now(),
			}},
		}
		logs, err := scanAuditLogs(rows)
		if err != nil {
			t.Fatal(err)
		}
		if logs[0].Details == nil {
			t.Fatal("expected non-nil Details")
		}
		if logs[0].Details["key"] != "value" {
			t.Fatalf("Details[key] = %v, want %v", logs[0].Details["key"], "value")
		}
		if logs[0].Details["count"] != float64(42) {
			t.Fatalf("Details[count] = %v, want %v", logs[0].Details["count"], float64(42))
		}
	})

	t.Run("multiple rows", func(t *testing.T) {
		count := 3
		var inputRows []mockAuditLogRow
		for range count {
			inputRows = append(inputRows, mockAuditLogRow{
				id: uuid.NewString(), userID: uuid.NewString(),
				action: "test.action", resourceType: "test", resourceID: uuid.NewString(),
				createdAt: time.Now(),
			})
		}
		rows := &mockAuditLogRows{rows: inputRows}
		logs, err := scanAuditLogs(rows)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) != count {
			t.Fatalf("expected %d logs, got %d", count, len(logs))
		}
		if logs[0].ID != inputRows[0].id {
			t.Fatalf("first row ID mismatch")
		}
		if logs[2].ID != inputRows[2].id {
			t.Fatalf("last row ID mismatch")
		}
	})

	t.Run("scan error propagated", func(t *testing.T) {
		rows := &mockAuditLogRows{
			rows: []mockAuditLogRow{{
				id: "1", userID: "u", action: "a",
				resourceType: "t", resourceID: "r",
				createdAt: time.Now(),
			}},
			scanErr: errors.New("scan failed"),
		}
		_, err := scanAuditLogs(rows)
		if err == nil || err.Error() != "scan failed" {
			t.Fatalf("expected scan error, got %v", err)
		}
	})

	t.Run("rows.Err propagated", func(t *testing.T) {
		rows := &mockAuditLogRows{
			rows: []mockAuditLogRow{{
				id: "1", userID: "u", action: "a",
				resourceType: "t", resourceID: "r",
				createdAt: time.Now(),
			}},
			rowsErr: errors.New("iteration error"),
		}
		_, err := scanAuditLogs(rows)
		if err == nil || err.Error() != "iteration error" {
			t.Fatalf("expected iteration error, got %v", err)
		}
	})

	t.Run("empty details JSON unmarshals to nil", func(t *testing.T) {
		rows := &mockAuditLogRows{
			rows: []mockAuditLogRow{{
				id: uuid.NewString(), userID: uuid.NewString(),
				action: "test", resourceType: "test", resourceID: uuid.NewString(),
				detailsJSON: []byte(""),
				createdAt:   time.Now(),
			}},
		}
		logs, err := scanAuditLogs(rows)
		if err != nil {
			t.Fatal(err)
		}
		if logs[0].Details != nil {
			t.Fatal("expected nil Details for empty JSON bytes")
		}
	})
}

func auditLogTestStore(t *testing.T) *Store {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "audit_log_test_" + uuid.NewString()[:8]
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(cleanupCtx, `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
	})
	if _, err := pool.Exec(ctx, `
		CREATE TABLE audit_logs (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL,
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT,
			details JSONB DEFAULT '{}'::jsonb,
			ip_address TEXT,
			user_agent TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		t.Fatal(err)
	}
	return &Store{db: pool, secrets: newTestKeyring()}
}

func TestAuditLogCreate(t *testing.T) {
	s := auditLogTestStore(t)
	ctx := context.Background()

	log := &models.AuditLog{
		UserID:       uuid.NewString(),
		Action:       "server.create",
		ResourceType: "server",
		ResourceID:   uuid.NewString(),
		Details:      models.JSONMap{"name": "test-server"},
		IPAddress:    "192.168.1.1",
		UserAgent:    "go-test/1.0",
	}
	if err := s.CreateAuditLog(ctx, log); err != nil {
		t.Fatal(err)
	}
	if log.ID == "" {
		t.Fatal("expected non-empty ID after CreateAuditLog")
	}
	if log.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt")
	}

	var storedID string
	var storedCreatedAt time.Time
	if err := s.db.QueryRow(ctx, `SELECT id::text, created_at FROM audit_logs WHERE id = $1`, log.ID).Scan(&storedID, &storedCreatedAt); err != nil {
		t.Fatal(err)
	}
	if storedID != log.ID {
		t.Fatalf("stored ID = %q, want %q", storedID, log.ID)
	}
}

func TestAuditLogCreatePreservesID(t *testing.T) {
	s := auditLogTestStore(t)
	ctx := context.Background()

	providedID := uuid.NewString()
	log := &models.AuditLog{
		UUIDModel:    models.UUIDModel{ID: providedID},
		UserID:       uuid.NewString(),
		Action:       "server.delete",
		ResourceType: "server",
		ResourceID:   uuid.NewString(),
		CreatedAt:    time.Now().Truncate(time.Microsecond),
	}
	if err := s.CreateAuditLog(ctx, log); err != nil {
		t.Fatal(err)
	}
	if log.ID != providedID {
		t.Fatalf("ID = %q, want %q", log.ID, providedID)
	}

	var storedID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM audit_logs WHERE id = $1`, providedID).Scan(&storedID); err != nil {
		t.Fatal(err)
	}
}

func TestAuditLogListByUser(t *testing.T) {
	s := auditLogTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	otherUserID := uuid.NewString()
	for range 5 {
		if err := s.CreateAuditLog(ctx, &models.AuditLog{
			UserID: userID, Action: "test.action",
			ResourceType: "server", ResourceID: uuid.NewString(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateAuditLog(ctx, &models.AuditLog{
		UserID: otherUserID, Action: "other.action",
		ResourceType: "server", ResourceID: uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("all logs for user", func(t *testing.T) {
		logs, err := s.ListAuditLogsByUser(ctx, userID, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) != 5 {
			t.Fatalf("expected 5 logs, got %d", len(logs))
		}
		for _, l := range logs {
			if l.UserID != userID {
				t.Fatalf("unexpected user %q in result", l.UserID)
			}
		}
	})

	t.Run("pagination limit", func(t *testing.T) {
		logs, err := s.ListAuditLogsByUser(ctx, userID, 2, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) != 2 {
			t.Fatalf("expected 2 logs, got %d", len(logs))
		}
	})

	t.Run("pagination offset", func(t *testing.T) {
		first, err := s.ListAuditLogsByUser(ctx, userID, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		offset, err := s.ListAuditLogsByUser(ctx, userID, 10, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(offset) != 2 {
			t.Fatalf("expected 2 logs with offset 3, got %d", len(offset))
		}
		if offset[0].ID != first[3].ID {
			t.Fatalf("offset result does not match: first ID at index 3 = %q, offset[0] = %q", first[3].ID, offset[0].ID)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		logs, err := s.ListAuditLogsByUser(ctx, uuid.NewString(), 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) != 0 {
			t.Fatalf("expected 0 logs, got %d", len(logs))
		}
	})
}

func TestAuditLogListByResource(t *testing.T) {
	s := auditLogTestStore(t)
	ctx := context.Background()

	resourceID := uuid.NewString()
	for range 3 {
		if err := s.CreateAuditLog(ctx, &models.AuditLog{
			UserID: uuid.NewString(), Action: "server.action",
			ResourceType: "server", ResourceID: resourceID,
		}); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("all logs for resource", func(t *testing.T) {
		logs, err := s.ListAuditLogsByResource(ctx, "server", resourceID, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) != 3 {
			t.Fatalf("expected 3 logs, got %d", len(logs))
		}
		for _, l := range logs {
			if l.ResourceType != "server" || l.ResourceID != resourceID {
				t.Fatalf("unexpected resource %s/%s", l.ResourceType, l.ResourceID)
			}
		}
	})

	t.Run("filter by different resource type", func(t *testing.T) {
		logs, err := s.ListAuditLogsByResource(ctx, "node", resourceID, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) != 0 {
			t.Fatalf("expected 0 logs for node resource, got %d", len(logs))
		}
	})

	t.Run("empty results", func(t *testing.T) {
		logs, err := s.ListAuditLogsByResource(ctx, "server", uuid.NewString(), 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) != 0 {
			t.Fatalf("expected 0 logs, got %d", len(logs))
		}
	})
}

func TestAuditLogListRecent(t *testing.T) {
	s := auditLogTestStore(t)
	ctx := context.Background()

	for range 10 {
		if err := s.CreateAuditLog(ctx, &models.AuditLog{
			UserID: uuid.NewString(), Action: "test.action",
			ResourceType: "server", ResourceID: uuid.NewString(),
		}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}

	t.Run("respects limit", func(t *testing.T) {
		logs, err := s.ListRecentAuditLogs(ctx, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) != 3 {
			t.Fatalf("expected 3 logs, got %d", len(logs))
		}
	})

	t.Run("ordered by created_at descending", func(t *testing.T) {
		logs, err := s.ListRecentAuditLogs(ctx, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) < 2 {
			t.Fatal("need at least 2 logs for ordering check")
		}
		for i := 1; i < len(logs); i++ {
			if logs[i].CreatedAt.After(logs[i-1].CreatedAt) {
				t.Fatalf("logs not in descending order at index %d: %v before %v", i, logs[i-1].CreatedAt, logs[i].CreatedAt)
			}
		}
	})

	t.Run("empty result set", func(t *testing.T) {
		logs, err := s.ListRecentAuditLogs(ctx, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(logs) != 10 {
			t.Fatalf("expected 10 logs, got %d", len(logs))
		}
	})
}

func TestAuditLogDelete(t *testing.T) {
	s := auditLogTestStore(t)
	ctx := context.Background()

	log := &models.AuditLog{
		UserID: uuid.NewString(), Action: "test.action",
		ResourceType: "server", ResourceID: uuid.NewString(),
	}
	if err := s.CreateAuditLog(ctx, log); err != nil {
		t.Fatal(err)
	}

	t.Run("delete existing log", func(t *testing.T) {
		if err := s.DeleteAuditLog(ctx, log.ID); err != nil {
			t.Fatal(err)
		}
		var exists bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_logs WHERE id = $1)`, log.ID).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatal("expected log to be deleted")
		}
	})

	t.Run("delete non-existent log does not error", func(t *testing.T) {
		if err := s.DeleteAuditLog(ctx, uuid.NewString()); err != nil {
			t.Fatalf("expected no error deleting non-existent log, got %v", err)
		}
	})
}
