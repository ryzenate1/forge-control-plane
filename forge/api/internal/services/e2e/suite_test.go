//go:build e2e

package e2e

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/placement"
	"gamepanel/forge/internal/services/heartbeatmonitor"
	"gamepanel/forge/internal/services/replicamanager"
	"gamepanel/forge/internal/services/reservations"
	scheduler2 "gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/store"
)

// TestSuite holds the shared test infrastructure
type TestSuite struct {
	DB               *pgxpool.Pool
	Store            *store.Store
	Scheduler        *scheduler2.Scheduler
	ReplicaMgr       *replicamanager.Manager
	Reservations     *reservations.Manager
	HeartbeatMonitor *heartbeatmonitor.Service
	Publisher        *mockPublisher
	Logger           *slog.Logger
	Schema           string
	LocationID       string
	Cleanup          func()
}

// mockPublisher implements events.Publisher for testing
type mockPublisher struct {
	mu         sync.Mutex
	events     []events.Envelope
	publishErr error
}

func (m *mockPublisher) Publish(ctx context.Context, envelope events.Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.publishErr != nil {
		return m.publishErr
	}
	m.events = append(m.events, envelope)
	return nil
}

func (m *mockPublisher) Events() []events.Envelope {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]events.Envelope, len(m.events))
	copy(result, m.events)
	return result
}

func (m *mockPublisher) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = nil
}

// SetupTestSuite creates a fresh test environment for each test
func SetupTestSuite(t *testing.T) *TestSuite {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	schema := "e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}

	cleanup := func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		_, _ = admin.Exec(cleanCtx, `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
		cancel()
	}

	schemaURL := databaseURL
	if strings.Contains(databaseURL, "?") {
		schemaURL += "&search_path=" + schema
	} else {
		schemaURL += "?search_path=" + schema
	}

	s, err := store.ConnectWithKeyring(ctx, schemaURL, nil)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}

	if err := s.RunMigrations(ctx, "../../../migrations"); err != nil {
		cleanup()
		t.Fatal(err)
	}

	rawCfg, err := pgxpool.ParseConfig(schemaURL)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	raw, err := pgxpool.NewWithConfig(ctx, rawCfg)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}

	// Close raw connection on cleanup
	originalCleanup := cleanup
	cleanup = func() {
		raw.Close()
		originalCleanup()
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	engine := placement.NewEngine(
		placement.NewScorer(placement.StrategyLeastLoaded),
		placement.NewConstraintChecker(),
	)

	publisher := &mockPublisher{}

	scheduler := scheduler2.New(s, engine, publisher)
	reservationMgr := reservations.New(s)
	replicaMgr := replicamanager.New(s, engine, scheduler, reservationMgr, nil, nil, nil, publisher)

	loc, err := s.CreateLocation(ctx, store.CreateLocationRequest{
		Short: schema,
		Long:  "E2E Test Location",
	}, nil)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}

	hmConfig := heartbeatmonitor.DefaultConfig()
	hmConfig.WarningThreshold = 5 * time.Second
	hmConfig.OfflineThreshold = 10 * time.Second
	heartbeatMonitor := heartbeatmonitor.NewWithConfig(s, publisher, hmConfig)

	return &TestSuite{
		DB:               raw,
		Store:            s,
		Scheduler:        scheduler,
		ReplicaMgr:       replicaMgr,
		Reservations:     reservationMgr,
		HeartbeatMonitor: heartbeatMonitor,
		Publisher:        publisher,
		Logger:           logger,
		Schema:           schema,
		LocationID:       loc.ID,
		Cleanup:          cleanup,
	}
}

// Helper functions for test data setup

func (s *TestSuite) CreateTestRegion(ctx context.Context, t *testing.T) string {
	t.Helper()
	regionID := uuid.NewString()
	err := s.Store.CreateRegion(ctx, store.CreateRegionRequest{
		ID:          regionID,
		UUID:        regionID,
		Name:        "Test Region",
		Slug:        "test-region",
		Description: "Test region for e2e",
		Enabled:     true,
	})
	require.NoError(t, err)
	return regionID
}

func (s *TestSuite) CreateTestNode(ctx context.Context, t *testing.T, regionID string, cpu, memory, disk int) string {
	t.Helper()
	node, _, err := s.Store.CreateNode(ctx, store.CreateNodeRequest{
		Name:       "test-node-" + uuid.NewString()[:8],
		RegionID:   regionID,
		LocationID: s.LocationID,
		MemoryMB:   memory,
		DiskMB:     disk,
		DaemonBase: "/tmp/e2e-test",
		BaseURL:    "http://localhost:8080",
		Scheme:     "http",
	}, nil)
	require.NoError(t, err)

	now := time.Now().UTC()
	_, err = s.DB.Exec(ctx, `
		UPDATE nodes
		SET status = 'online',
		    desired_state = 'active',
		    actual_state = 'online',
		    heartbeat_state = 'healthy',
		    last_seen_at = $1,
		    runtime_provider = 'docker'
		WHERE id = $2
	`, now, node.ID)
	require.NoError(t, err)

	_, err = s.Store.CreateNodeCapacitySnapshot(ctx, store.NodeCapacitySnapshot{
		NodeID:          node.ID,
		RegionID:        regionID,
		AllocatedCPU:    0,
		AvailableCPU:    cpu,
		AllocatedMemory: 0,
		AvailableMemory: memory,
		AllocatedDisk:   0,
		AvailableDisk:   disk,
		ServerCount:     0,
		UpdatedAt:       now,
	})
	require.NoError(t, err)

	return node.ID
}

func (s *TestSuite) CreateTestApplication(ctx context.Context, t *testing.T, name string, replicas int) store.ReplicaApplication {
	t.Helper()
	app, err := s.Store.CreateReplicaApp(ctx, store.CreateReplicaAppRequest{
		Name:            name,
		Replicas:        replicas,
		CPU:             1024,
		MemoryMB:        2048,
		DiskMB:          10240,
		RuntimeProvider: "docker",
	})
	require.NoError(t, err)
	return app
}

func (s *TestSuite) CreateTestServer(ctx context.Context, t *testing.T, name, nodeID, regionID string) store.Server {
	t.Helper()
	serverID := uuid.NewString()
	serverUUID := uuid.NewString()
	_, err := s.DB.Exec(ctx, `
		INSERT INTO servers (
			id, uuid, uuid_short, node_id, owner_id, template_id, egg_id,
			name, status, desired_state, actual_state,
			memory_mb, cpu_shares, cpu_limit, disk_mb,
			database_limit, backup_limit, allocation_limit,
			io_weight, swap_mb, threads, oom_disabled, docker_image,
			startup_command, primary_allocation_id, installed, config_sync_pending,
			skip_scripts, docker_labels
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7,
			$8, 'running', 'running', 'running',
			$9, $10, $11, $12,
			$13, $14, $15,
			$16, $17, $18, $19, $20,
			$21, $22, false, false,
			$23, '{}'::jsonb)
	`, serverID, serverUUID, serverUUID[:8], nodeID,
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000002",
		"00000000-0000-0000-0000-000000000002",
		name,
		1024, 1024, 0, 10240,
		0, 0, 0,
		500, 0, "", false, "test-image:latest",
		"echo hello", "", false,
	)
	require.NoError(t, err)

	return store.Server{ID: serverID, Node: nodeID, Name: name}
}

func (s *TestSuite) CreateTestHeartbeatHistory(ctx context.Context, nodeID string, success bool, observedAt time.Time) {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO node_heartbeat_history (
			id, node_id, observed_at, success
		)
		VALUES ($1, $2, $3, $4)
	`, uuid.NewString(), nodeID, observedAt, success)
	if err != nil {
		panic(err)
	}
}

func (s *TestSuite) AddNodeHeartbeatHistoryForRecovery(ctx context.Context, nodeID string, count int) {
	now := time.Now().UTC()
	for i := 0; i < count; i++ {
		s.CreateTestHeartbeatHistory(ctx, nodeID, true, now.Add(-time.Duration(i)*time.Second))
	}
}
