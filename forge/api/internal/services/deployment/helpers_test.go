package deployment

import (
	"context"
	"os"
	"testing"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testService(t *testing.T) (*Service, func()) {
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
	schema := "deployment_rev_test_" + uuid.NewString()[:8]
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

	if err := createTables(ctx, pool); err != nil {
		t.Fatal(err)
	}

	st := store.NewWithPool(pool)

	return New(st), func() {}
}

func createTestDeployment(t *testing.T, svc *Service, serverID string, strategy Strategy) *Deployment {
	t.Helper()
	now := time.Now().UTC()
	dep := &Deployment{
		ID:           uuid.NewString(),
		ServerID:     serverID,
		Strategy:     strategy,
		Status:       StatusCompleted,
		Image:        "nginx:1.25",
		BlueTargetID: serverID + "-blue",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err := svc.store.CreateDeployment(t.Context(), toStoreDeployment(dep))
	if err != nil {
		t.Fatalf("create test deployment: %v", err)
	}
	return dep
}

func createTables(ctx context.Context, pool *pgxpool.Pool) error {
	stmts := []string{
		`CREATE TABLE deployments (
			id UUID PRIMARY KEY,
			server_id UUID NOT NULL,
			strategy TEXT NOT NULL DEFAULT 'blue-green',
			status TEXT NOT NULL DEFAULT 'pending',
			image TEXT NOT NULL,
			blue_target_id TEXT NOT NULL DEFAULT '',
			green_target_id TEXT NOT NULL DEFAULT '',
			active_target TEXT NOT NULL DEFAULT 'blue',
			health_check_path TEXT NOT NULL DEFAULT '',
			health_check_port INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			current_revision_id UUID,
			rollout_strategy TEXT NOT NULL DEFAULT 'recreate',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			completed_at TIMESTAMPTZ
		)`,
		`CREATE TABLE deployment_revisions (
			id UUID PRIMARY KEY,
			deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
			revision_number INTEGER NOT NULL,
			image_ref TEXT NOT NULL DEFAULT '',
			compose_manifest_ref TEXT NOT NULL DEFAULT '',
			git_commit_sha TEXT NOT NULL DEFAULT '',
			config_hash TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			deployed_at TIMESTAMPTZ,
			description TEXT NOT NULL DEFAULT '',
			metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	}
	for _, stmt := range stmts {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
