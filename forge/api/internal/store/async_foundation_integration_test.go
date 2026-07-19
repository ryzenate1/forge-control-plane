package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func asyncTestStore(t *testing.T) (*Store, context.Context) {
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
	schema := "async_test_" + uuid.NewString()[:8]
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); _, _ = admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); admin.Close() })
	ddl := `
		CREATE TABLE users (id UUID PRIMARY KEY, email TEXT NOT NULL, disabled BOOLEAN NOT NULL DEFAULT FALSE);
		CREATE TABLE password_reset_tokens (id UUID PRIMARY KEY, user_id UUID REFERENCES users(id), token_hash TEXT, expires_at TIMESTAMPTZ, requested_ip TEXT);
		CREATE TABLE mail_outbox (id UUID PRIMARY KEY, recipient TEXT, subject TEXT, text_body TEXT, html_body TEXT, attempts INT DEFAULT 0, next_attempt_at TIMESTAMPTZ DEFAULT now(), last_error TEXT, sent_at TIMESTAMPTZ, locked_at TIMESTAMPTZ, locked_by TEXT, created_at TIMESTAMPTZ DEFAULT now());
		CREATE TABLE servers (id UUID PRIMARY KEY, status TEXT NOT NULL);
		CREATE TABLE server_schedules (id UUID PRIMARY KEY, server_id UUID REFERENCES servers(id), name TEXT, cron_minute TEXT, cron_hour TEXT, cron_day_of_month TEXT, cron_month TEXT, cron_day_of_week TEXT, only_when_online BOOLEAN, enabled BOOLEAN, last_run_at TIMESTAMPTZ, next_run_at TIMESTAMPTZ, created_at TIMESTAMPTZ DEFAULT now(), updated_at TIMESTAMPTZ DEFAULT now(), lease_owner TEXT, lease_expires_at TIMESTAMPTZ);
		CREATE TABLE schedule_tasks (id UUID PRIMARY KEY, schedule_id UUID REFERENCES server_schedules(id), sequence INT, action TEXT, payload JSONB, time_offset_seconds INT, continue_on_failure BOOLEAN, created_at TIMESTAMPTZ DEFAULT now());
		CREATE TABLE schedule_runs (id UUID PRIMARY KEY, schedule_id UUID REFERENCES server_schedules(id), server_id UUID REFERENCES servers(id), status TEXT, trigger TEXT, error TEXT, started_at TIMESTAMPTZ DEFAULT now(), finished_at TIMESTAMPTZ, worker_id TEXT, lease_expires_at TIMESTAMPTZ, recovered_from_run_id UUID);
	`
	if _, err := pool.Exec(ctx, ddl); err != nil {
		t.Fatal(err)
	}
	return &Store{db: pool, secrets: newTestKeyring()}, ctx
}

func TestPasswordResetTokenAndMailEnqueueAtomically(t *testing.T) {
	s, ctx := asyncTestStore(t)
	userID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id,email) VALUES ($1,'person@example.com')`, userID); err != nil {
		t.Fatal(err)
	}
	accepted, err := s.EnqueuePasswordReset(ctx, "person@example.com", "hash", 30*time.Minute, "192.0.2.1", "https://panel.example/reset?token=x")
	if err != nil || !accepted {
		t.Fatalf("enqueue = %v, %v", accepted, err)
	}
	var tokens, mails int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM password_reset_tokens`).Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM mail_outbox`).Scan(&mails); err != nil {
		t.Fatal(err)
	}
	if tokens != 1 || mails != 1 {
		t.Fatalf("tokens=%d mails=%d", tokens, mails)
	}
	accepted, err = s.EnqueuePasswordReset(ctx, "missing@example.com", "other", time.Minute, "", "https://panel.example/reset")
	if err != nil || accepted {
		t.Fatalf("unknown account acceptance = %v, %v", accepted, err)
	}
}

func TestScheduleClaimLeaseAndOnlyWhenOnline(t *testing.T) {
	s, ctx := asyncTestStore(t)
	onlineID, offlineID := uuid.NewString(), uuid.NewString()
	dueID, offlineScheduleID := uuid.NewString(), uuid.NewString()
	_, err := s.db.Exec(ctx, `INSERT INTO servers(id,status) VALUES($1,'running'),($2,'offline'); INSERT INTO server_schedules(id,server_id,name,cron_minute,cron_hour,cron_day_of_month,cron_month,cron_day_of_week,only_when_online,enabled,next_run_at) VALUES($3,$1,'due','*','*','*','*','*',true,true,now()-interval '1 minute'),($4,$2,'offline','*','*','*','*','*',true,true,now()-interval '1 minute')`, onlineID, offlineID, dueID, offlineScheduleID)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	claims := make(chan *ClaimedSchedule, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(worker int) {
			defer wg.Done()
			claim, err := s.ClaimDueSchedule(ctx, time.Now(), fmt.Sprintf("worker-%d", worker))
			claims <- claim
			errs <- err
		}(i)
	}
	wg.Wait()
	close(claims)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for claim := range claims {
		if claim != nil {
			count++
			if claim.Schedule.ID != dueID {
				t.Fatalf("claimed offline schedule %s", claim.Schedule.ID)
			}
		}
	}
	if count != 1 {
		t.Fatalf("claims=%d, want 1", count)
	}
	claim, err := s.ClaimDueSchedule(ctx, time.Now(), "worker-3")
	if err != nil {
		t.Fatal(err)
	}
	if claim != nil {
		t.Fatal("offline-only schedule must not be claimed")
	}
}
