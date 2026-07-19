package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gamepanel/forge/internal/secrets"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Store struct {
	db      *pgxpool.Pool
	secrets *secrets.Keyring
}

const (
	ScheduleTaskActionPower   ScheduleTaskAction = "power"
	ScheduleTaskActionBackup  ScheduleTaskAction = "backup"
	ScheduleTaskActionCommand ScheduleTaskAction = "command"
)

// ToDTO converts ServerControlTarget to safe DTO
func (t ServerControlTarget) ToDTO() ServerControlTargetDTO {
	return ServerControlTargetDTO{
		ServerID: t.ServerID,
		NodeURL:  t.NodeURL,
	}
}

// ToDTO converts ServerProvisionTarget to safe DTO
func (t ServerProvisionTarget) ToDTO() ServerProvisionTargetDTO {
	return ServerProvisionTargetDTO{
		ServerID:          t.ServerID,
		EggID:             t.EggID,
		Name:              t.Name,
		NodeURL:           t.NodeURL,
		Image:             t.Image,
		StartupCommand:    t.StartupCommand,
		InstallScript:     t.InstallScript,
		InstallContainer:  t.InstallContainer,
		InstallEntrypoint: t.InstallEntrypoint,
		ConfigJSON:        t.ConfigJSON,
		FileDenylist:      t.FileDenylist,
		Environment:       t.Environment,
		Mounts:            t.Mounts,
		MemoryMB:          t.MemoryMB,
		SwapMB:            t.SwapMB,
		CPUShares:         t.CPUShares,
		CPULimit:          t.CPULimit,
		DiskMB:            t.DiskMB,
		IOWeight:          t.IOWeight,
		Threads:           t.Threads,
		OOMDisabled:       t.OOMDisabled,
		AllocationIP:      t.AllocationIP,
		AllocationPort:    t.AllocationPort,
		Allocations:       t.Allocations,
		Suspended:         t.Suspended,
		Installed:         t.Installed,
		Status:            t.Status,
		SkipScripts:       t.SkipScripts,
		DockerLabels:      t.DockerLabels,
	}
}

const (
	ScheduleRunRunning ScheduleRunStatus = "running"
	ScheduleRunSuccess ScheduleRunStatus = "success"
	ScheduleRunFailed  ScheduleRunStatus = "failed"
	ScheduleRunPartial ScheduleRunStatus = "partial"
	ScheduleRunSkipped ScheduleRunStatus = "skipped"
)

const (
	ScheduleTaskRunSuccess ScheduleTaskRunStatus = "success"
	ScheduleTaskRunFailed  ScheduleTaskRunStatus = "failed"
	ScheduleTaskRunSkipped ScheduleTaskRunStatus = "skipped"
)

func Connect(ctx context.Context, databaseURL string) (*Store, error) {
	return ConnectWithKeyring(ctx, databaseURL, nil)
}

func ConnectWithKeyring(ctx context.Context, databaseURL string, keyring *secrets.Keyring) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 8
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour

	var lastErr error
	for attempt := 0; attempt < 20; attempt++ {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if pingErr := pool.Ping(ctx); pingErr == nil {
				return &Store{db: pool, secrets: keyring}, nil
			} else {
				lastErr = pingErr
			}
			pool.Close()
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("connect postgres: %w", lastErr)
}

func (s *Store) Close() {
	if s != nil && s.db != nil {
		s.db.Close()
	}
}

func (s *Store) DB() *pgxpool.Pool {
	return s.db
}

func isValidScheduleTaskAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case string(ScheduleTaskActionPower), string(ScheduleTaskActionBackup), string(ScheduleTaskActionCommand):
		return true
	default:
		return false
	}
}

func (s *Store) RunMigrations(ctx context.Context, dir string) error {
	if _, err := s.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var applied bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, name).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		tx, err := s.db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		for _, statement := range splitSQLStatements(string(body)) {
			if _, err := tx.Exec(ctx, statement); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("run migration %s: %w", name, err)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) Seed(ctx context.Context) error {
	adminID := "11111111-1111-1111-1111-111111111111"
	nodeID := "22222222-2222-2222-2222-222222222222"
	templateID := "33333333-3333-3333-3333-333333333333"
	serverID := "44444444-4444-4444-4444-444444444444"
	allocationID := "55555555-5555-5555-5555-555555555555"
	spareAllocationID := "66666666-6666-6666-6666-666666666666"

	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if _, err = s.db.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, role)
		VALUES ($1, 'admin@example.com', $2, 'admin')
		ON CONFLICT (email) DO NOTHING
	`, adminID, string(hash)); err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, r.id FROM roles r WHERE r.key = 'admin'
		ON CONFLICT (user_id, role_id) DO NOTHING
	`, adminID); err != nil {
		return err
	}
	nodeToken := "dev-node-token"
	nodeTokenEncrypted, err := s.encryptSecret(nodeToken, secretAAD("nodes", nodeID, "daemon_token"))
	if err != nil {
		return err
	}
	nodeTokenHash, err := bcrypt.GenerateFromPassword([]byte(nodeToken), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		INSERT INTO nodes (
			id, uuid, name, region, base_url, fqdn, scheme, status, token_hash,
			daemon_token_id, daemon_token, daemon_token_encrypted, daemon_listen, daemon_sftp, daemon_base, last_seen_at
		)
		VALUES ($1, $1, 'Ubuntu Demo Node', 'local-lab', 'http://daemon:9090', 'daemon', 'http', 'online',
		        $2, 'devnodetoken0001', '', $3, 9090, 2022, '/srv/game-panel/servers', now())
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			token_hash = EXCLUDED.token_hash,
			daemon_token_id = EXCLUDED.daemon_token_id,
			daemon_token = '',
			daemon_token_encrypted = EXCLUDED.daemon_token_encrypted,
			last_seen_at = EXCLUDED.last_seen_at
	`, nodeID, string(nodeTokenHash), nodeTokenEncrypted); err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		INSERT INTO eggs (id, nest_id, name, description, docker_images, startup, config, default_memory_mb)
		SELECT $1, id, 'Minecraft Java', '', jsonb_build_object('Java', 'itzg/minecraft-server:latest'), '', '{}'::jsonb, 2048
		FROM nests WHERE name = 'Games'
		ON CONFLICT (id) DO UPDATE SET
			docker_images = EXCLUDED.docker_images,
			startup = EXCLUDED.startup,
			default_memory_mb = EXCLUDED.default_memory_mb
	`, templateID); err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		INSERT INTO servers (id, node_id, owner_id, template_id, egg_id, name, status, memory_mb, cpu_shares, disk_mb)
		VALUES ($1, $2, $3, $4, $4, 'Survival SMP', 'stopped', 2048, 1024, 10240)
		ON CONFLICT (id) DO NOTHING
	`, serverID, nodeID, adminID, templateID); err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		INSERT INTO allocations (id, node_id, server_id, ip, port, alias, notes)
		VALUES ($1, $2, $3, '0.0.0.0', 25565, 'minecraft.local', 'default Minecraft Java allocation')
		ON CONFLICT (node_id, ip, port) DO UPDATE SET server_id = EXCLUDED.server_id, alias = EXCLUDED.alias
	`, allocationID, nodeID, serverID); err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		UPDATE servers SET primary_allocation_id = $1 WHERE id = $2 AND (primary_allocation_id IS NULL OR primary_allocation_id != $1)
	`, allocationID, serverID); err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		INSERT INTO allocations (id, node_id, server_id, ip, port, alias, notes)
		VALUES ($1, $2, NULL, '0.0.0.0', 25566, 'minecraft-alt.local', 'spare Minecraft Java allocation')
		ON CONFLICT (node_id, ip, port) DO NOTHING
	`, spareAllocationID, nodeID); err != nil {
		return err
	}

	return s.AppendAudit(ctx, &adminID, "seeded prototype data", "system", nil, `{"source":"startup"}`)
}

const (
	EvacuationPlanStatusPending   EvacuationPlanStatus = "pending"
	EvacuationPlanStatusRunning   EvacuationPlanStatus = "running"
	EvacuationPlanStatusCompleted EvacuationPlanStatus = "completed"
	EvacuationPlanStatusCancelled EvacuationPlanStatus = "cancelled"
	EvacuationPlanStatusFailed    EvacuationPlanStatus = "failed"
)

const (
	NodeHeartbeatStateHealthy     NodeHeartbeatState = "healthy"
	NodeHeartbeatStateSuspected   NodeHeartbeatState = "suspected"
	NodeHeartbeatStateUnreachable NodeHeartbeatState = "unreachable"
	NodeHeartbeatStateOffline     NodeHeartbeatState = "offline"
	NodeHeartbeatStateRecovering  NodeHeartbeatState = "recovering"
)

const (
	MigrationStatusPending      MigrationStatus = "pending"
	MigrationStatusPlanned      MigrationStatus = "planned"
	MigrationStatusPreparing    MigrationStatus = "preparing"
	MigrationStatusTransferring MigrationStatus = "transferring"
	MigrationStatusRestoring    MigrationStatus = "restoring"
	MigrationStatusInProgress   MigrationStatus = "in_progress"
	MigrationStatusCompleted    MigrationStatus = "completed"
	MigrationStatusFailed       MigrationStatus = "failed"
	MigrationStatusCancelled    MigrationStatus = "cancelled"
)

const (
	RecoveryPlanStatusPending   RecoveryPlanStatus = "pending"
	RecoveryPlanStatusPlanning  RecoveryPlanStatus = "planning"
	RecoveryPlanStatusPlanned   RecoveryPlanStatus = "planned"
	RecoveryPlanStatusExecuting RecoveryPlanStatus = "executing"
	RecoveryPlanStatusCompleted RecoveryPlanStatus = "completed"
	// Restored means the verified backup was restored on the target daemon. It
	// deliberately does not imply server ownership or allocation migration.
	RecoveryPlanStatusRestored  RecoveryPlanStatus = "restored"
	RecoveryPlanStatusCancelled RecoveryPlanStatus = "cancelled"
	RecoveryPlanStatusFailed    RecoveryPlanStatus = "failed"
)

const (
	RecoveryItemStatusPending   RecoveryItemStatus = "pending"
	RecoveryItemStatusPlanned   RecoveryItemStatus = "planned"
	RecoveryItemStatusExecuting RecoveryItemStatus = "executing"
	RecoveryItemStatusCompleted RecoveryItemStatus = "completed"
	// Restored means backup data was restored, but no live migration was run.
	RecoveryItemStatusRestored  RecoveryItemStatus = "restored"
	RecoveryItemStatusCancelled RecoveryItemStatus = "cancelled"
	RecoveryItemStatusFailed    RecoveryItemStatus = "failed"
	RecoveryItemStatusSkipped   RecoveryItemStatus = "skipped"
)

const (
	PlacementReservationStatusPending   PlacementReservationStatus = "pending"
	PlacementReservationStatusActive    PlacementReservationStatus = "active"
	PlacementReservationStatusCompleted PlacementReservationStatus = "completed"
	PlacementReservationStatusExpired   PlacementReservationStatus = "expired"
	PlacementReservationStatusCancelled PlacementReservationStatus = "cancelled"
	PlacementReservationStatusCanceled  PlacementReservationStatus = "canceled"
	PlacementReservationStatusUsed      PlacementReservationStatus = "used"
)

const (
	PlacementReservationTypePlacement  PlacementReservationType = "placement"
	PlacementReservationTypeMigration  PlacementReservationType = "migration"
	PlacementReservationTypeEvacuation PlacementReservationType = "evacuation"
	PlacementReservationTypeRecovery   PlacementReservationType = "recovery"
)

const (
	ServerDesiredStateRunning    ServerDesiredState = "running"
	ServerDesiredStateStopped    ServerDesiredState = "stopped"
	ServerDesiredStateTerminated ServerDesiredState = "terminated"
)

const (
	ServerActualStateOffline         ServerActualState = "offline"
	ServerActualStateStarting        ServerActualState = "starting"
	ServerActualStateRunning         ServerActualState = "running"
	ServerActualStateStopping        ServerActualState = "stopping"
	ServerActualStateStopped         ServerActualState = "stopped"
	ServerActualStateInstalling      ServerActualState = "installing"
	ServerActualStateRestoringBackup ServerActualState = "restoring_backup"
	ServerActualStateCrashed         ServerActualState = "crashed"
	ServerActualStateTerminating     ServerActualState = "terminating"
	ServerActualStateTerminated      ServerActualState = "terminated"
	ServerActualStateUnknown         ServerActualState = "unknown"
)

const (
	NodeDesiredStateActive      NodeDesiredState = "active"
	NodeDesiredStateMaintenance NodeDesiredState = "maintenance"
	NodeDesiredStateDraining    NodeDesiredState = "draining"
)

const (
	NodeActualStateOnline   NodeActualState = "online"
	NodeActualStateDegraded NodeActualState = "degraded"
	NodeActualStateOffline  NodeActualState = "offline"
)
