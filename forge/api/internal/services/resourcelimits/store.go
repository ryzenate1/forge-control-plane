package resourcelimits

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the persistence contract the Service is written against. Keeping it
// an interface means the gating logic in ShouldRollback and the compose
// renderer can be driven from tests without a database; PostgresStore is the
// production implementation.
type Store interface {
	UpsertProcessConfig(ctx context.Context, cfg ProcessConfig) (ProcessConfig, error)
	ListProcessConfigs(ctx context.Context, applicationID string) ([]ProcessConfig, error)
	GetProcessConfig(ctx context.Context, applicationID, processType string) (ProcessConfig, error)
	DeleteProcessConfig(ctx context.Context, applicationID, processType string) (bool, error)
	// GetApplicationByServer backs the per-server view: process configuration is
	// owned by an application, but the screen that edits it is a server tab.
	GetApplicationByServer(ctx context.Context, serverID string) (ApplicationRef, error)

	InsertHealthObservation(ctx context.Context, obs HealthObservation) (HealthObservation, error)
	ListHealthObservationsByServer(ctx context.Context, serverID string, limit int) ([]HealthObservation, error)
	ListHealthObservationsByApplication(ctx context.Context, applicationID string, since time.Time) ([]HealthObservation, error)
	// PruneHealthObservations deletes rows older than cutoff and reports how
	// many. Observations are append-only per probe, so without this the table
	// grows forever.
	PruneHealthObservations(ctx context.Context, cutoff time.Time) (int64, error)
}

// PostgresStore implements Store over the shared connection pool, the same way
// the pipeline phase does, so this feature adds no files to internal/store.
type PostgresStore struct {
	db *pgxpool.Pool
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: db}
}

var _ Store = (*PostgresStore)(nil)

// applicationColumns is shared by every query that resolves an application so a
// column rename cannot leave one reader behind.
const applicationColumns = `id::text, name, COALESCE(server_id::text, ''), COALESCE(org_id::text, ''), source_type`

func (s *PostgresStore) UpsertProcessConfig(ctx context.Context, cfg ProcessConfig) (ProcessConfig, error) {
	if s.db == nil {
		return ProcessConfig{}, errors.New("resource limits store: postgres pool is not configured")
	}
	id := cfg.ID
	if id == "" {
		id = uuid.NewString()
	}
	var healthType any
	if cfg.HealthCheckType != HealthCheckNone {
		healthType = string(cfg.HealthCheckType)
	}
	tag, err := s.db.Exec(ctx, `
		INSERT INTO process_configs (
			id, application_id, process_type, cpu_limit, memory_limit, replicas,
			health_check_type, health_check_path, health_check_port, health_check_command,
			health_check_interval, health_check_timeout, health_check_retries,
			health_check_start_period, health_check_gating, enabled
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13,
			$14, $15, $16
		)
		ON CONFLICT (application_id, process_type) DO UPDATE SET
			cpu_limit = EXCLUDED.cpu_limit,
			memory_limit = EXCLUDED.memory_limit,
			replicas = EXCLUDED.replicas,
			health_check_type = EXCLUDED.health_check_type,
			health_check_path = EXCLUDED.health_check_path,
			health_check_port = EXCLUDED.health_check_port,
			health_check_command = EXCLUDED.health_check_command,
			health_check_interval = EXCLUDED.health_check_interval,
			health_check_timeout = EXCLUDED.health_check_timeout,
			health_check_retries = EXCLUDED.health_check_retries,
			health_check_start_period = EXCLUDED.health_check_start_period,
			health_check_gating = EXCLUDED.health_check_gating,
			enabled = EXCLUDED.enabled,
			updated_at = NOW()
	`,
		id, cfg.ApplicationID, cfg.ProcessType, cfg.CPULimit, cfg.MemoryLimit, cfg.Replicas,
		healthType, cfg.HealthCheckPath, cfg.HealthCheckPort, cfg.HealthCheckCommand,
		cfg.HealthCheckInterval, cfg.HealthCheckTimeout, cfg.HealthCheckRetries,
		cfg.HealthCheckStartPeriod, cfg.HealthCheckGating, cfg.Enabled,
	)
	if err != nil {
		return ProcessConfig{}, fmt.Errorf("upsert process config %q: %w", cfg.ProcessType, err)
	}
	if tag.RowsAffected() == 0 {
		// Postgres reports 0 here only when the row vanished mid-statement. A
		// silent success would let the caller believe limits were stored.
		return ProcessConfig{}, fmt.Errorf("upsert process config %q: no row was written", cfg.ProcessType)
	}
	return s.GetProcessConfig(ctx, cfg.ApplicationID, cfg.ProcessType)
}

const processConfigColumns = `
	id::text, application_id::text, process_type, cpu_limit, memory_limit, replicas,
	COALESCE(health_check_type, ''), COALESCE(health_check_path, ''), health_check_port,
	COALESCE(health_check_command, ''), health_check_interval, health_check_timeout,
	health_check_retries, health_check_start_period, health_check_gating, enabled,
	created_at, updated_at`

func scanProcessConfig(row pgx.Row) (ProcessConfig, error) {
	var cfg ProcessConfig
	err := row.Scan(
		&cfg.ID, &cfg.ApplicationID, &cfg.ProcessType, &cfg.CPULimit, &cfg.MemoryLimit, &cfg.Replicas,
		&cfg.HealthCheckType, &cfg.HealthCheckPath, &cfg.HealthCheckPort,
		&cfg.HealthCheckCommand, &cfg.HealthCheckInterval, &cfg.HealthCheckTimeout,
		&cfg.HealthCheckRetries, &cfg.HealthCheckStartPeriod, &cfg.HealthCheckGating, &cfg.Enabled,
		&cfg.CreatedAt, &cfg.UpdatedAt,
	)
	return cfg, err
}

func (s *PostgresStore) ListProcessConfigs(ctx context.Context, applicationID string) ([]ProcessConfig, error) {
	if s.db == nil {
		return nil, errors.New("resource limits store: postgres pool is not configured")
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+processConfigColumns+`
		FROM process_configs WHERE application_id = $1 ORDER BY process_type
	`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list process configs: %w", err)
	}
	defer rows.Close()

	configs := make([]ProcessConfig, 0, 4)
	for rows.Next() {
		cfg, err := scanProcessConfig(rows)
		if err != nil {
			return nil, fmt.Errorf("scan process config: %w", err)
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

func (s *PostgresStore) GetProcessConfig(ctx context.Context, applicationID, processType string) (ProcessConfig, error) {
	if s.db == nil {
		return ProcessConfig{}, errors.New("resource limits store: postgres pool is not configured")
	}
	cfg, err := scanProcessConfig(s.db.QueryRow(ctx, `
		SELECT `+processConfigColumns+`
		FROM process_configs WHERE application_id = $1 AND process_type = $2
	`, applicationID, processType))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProcessConfig{}, ErrProcessConfigNotFound
		}
		return ProcessConfig{}, fmt.Errorf("get process config %q: %w", processType, err)
	}
	return cfg, nil
}

func (s *PostgresStore) DeleteProcessConfig(ctx context.Context, applicationID, processType string) (bool, error) {
	if s.db == nil {
		return false, errors.New("resource limits store: postgres pool is not configured")
	}
	tag, err := s.db.Exec(ctx, `
		DELETE FROM process_configs WHERE application_id = $1 AND process_type = $2
	`, applicationID, processType)
	if err != nil {
		return false, fmt.Errorf("delete process config %q: %w", processType, err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *PostgresStore) GetApplicationByServer(ctx context.Context, serverID string) (ApplicationRef, error) {
	if s.db == nil {
		return ApplicationRef{}, errors.New("resource limits store: postgres pool is not configured")
	}
	var ref ApplicationRef
	err := s.db.QueryRow(ctx, `
		SELECT `+applicationColumns+`
		FROM applications WHERE server_id = $1 ORDER BY created_at ASC LIMIT 1
	`, serverID).Scan(&ref.ID, &ref.Name, &ref.ServerID, &ref.Organization, &ref.SourceType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ApplicationRef{}, ErrNoBindableApplication
		}
		return ApplicationRef{}, fmt.Errorf("resolve application for server %s: %w", serverID, err)
	}
	return ref, nil
}

func (s *PostgresStore) InsertHealthObservation(ctx context.Context, obs HealthObservation) (HealthObservation, error) {
	if s.db == nil {
		return HealthObservation{}, errors.New("resource limits store: postgres pool is not configured")
	}
	if obs.ServerID == "" {
		return HealthObservation{}, errors.New("health observations must name the server they came from")
	}
	if obs.ProcessType == "" {
		return HealthObservation{}, errors.New("health observations must name the process type they came from")
	}
	id := obs.ID
	if id == "" {
		id = uuid.NewString()
	}
	observedAt := obs.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	var applicationID any
	if obs.ApplicationID != nil && *obs.ApplicationID != "" {
		applicationID = *obs.ApplicationID
	}
	err := s.db.QueryRow(ctx, `
		INSERT INTO health_observations (id, server_id, application_id, process_type, healthy, detail, observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, server_id::text, application_id::text, process_type, healthy, detail, observed_at
	`, id, obs.ServerID, applicationID, obs.ProcessType, obs.Healthy, obs.Detail, observedAt,
	).Scan(&obs.ID, &obs.ServerID, &obs.ApplicationID, &obs.ProcessType, &obs.Healthy, &obs.Detail, &obs.ObservedAt)
	if err != nil {
		return HealthObservation{}, fmt.Errorf("record health observation: %w", err)
	}
	return obs, nil
}

const healthObservationColumns = `
	id::text, server_id::text, application_id::text, process_type, healthy, COALESCE(detail, ''), observed_at`

func scanHealthObservation(row pgx.Row) (HealthObservation, error) {
	var obs HealthObservation
	err := row.Scan(&obs.ID, &obs.ServerID, &obs.ApplicationID, &obs.ProcessType, &obs.Healthy, &obs.Detail, &obs.ObservedAt)
	return obs, err
}

func (s *PostgresStore) ListHealthObservationsByServer(ctx context.Context, serverID string, limit int) ([]HealthObservation, error) {
	if s.db == nil {
		return nil, errors.New("resource limits store: postgres pool is not configured")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+healthObservationColumns+`
		FROM health_observations WHERE server_id = $1
		ORDER BY observed_at DESC LIMIT $2
	`, serverID, limit)
	if err != nil {
		return nil, fmt.Errorf("list health observations for server %s: %w", serverID, err)
	}
	return collectObservations(rows)
}

func (s *PostgresStore) ListHealthObservationsByApplication(ctx context.Context, applicationID string, since time.Time) ([]HealthObservation, error) {
	if s.db == nil {
		return nil, errors.New("resource limits store: postgres pool is not configured")
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+healthObservationColumns+`
		FROM health_observations
		WHERE application_id = $1 AND observed_at >= $2
		ORDER BY observed_at DESC
	`, applicationID, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("list health observations for application %s: %w", applicationID, err)
	}
	return collectObservations(rows)
}

func collectObservations(rows pgx.Rows) ([]HealthObservation, error) {
	defer rows.Close()
	observations := make([]HealthObservation, 0, 16)
	for rows.Next() {
		obs, err := scanHealthObservation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan health observation: %w", err)
		}
		observations = append(observations, obs)
	}
	return observations, rows.Err()
}

func (s *PostgresStore) PruneHealthObservations(ctx context.Context, cutoff time.Time) (int64, error) {
	if s.db == nil {
		return 0, errors.New("resource limits store: postgres pool is not configured")
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM health_observations WHERE observed_at < $1`, cutoff.UTC())
	if err != nil {
		return 0, fmt.Errorf("prune health observations: %w", err)
	}
	return tag.RowsAffected(), nil
}
