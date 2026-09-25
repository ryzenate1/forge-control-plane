package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Backup-engine (restic / kopia) records. These tables back the
// services/backupengine service and the /admin/backup-engines API surface
// (migration 228_a_backup_engine.sql).

type BackupEngineRepository struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Engine            string          `json:"engine"`
	Location          string          `json:"location"`
	PasswordRef       string          `json:"passwordRef,omitempty"`
	PasswordEncrypted string          `json:"-"`
	Encryption        string          `json:"encryption"`
	PrunePolicy       json.RawMessage `json:"prunePolicy"`
	NodeID            *string         `json:"nodeId,omitempty"`
	ServerID          *string         `json:"serverId,omitempty"`
	ArtifactID        *string         `json:"artifactId,omitempty"`
	Initialized       bool            `json:"initialized"`
	LastSnapshotAt    *time.Time      `json:"lastSnapshotAt,omitempty"`
	NextRunAt         *time.Time      `json:"nextRunAt,omitempty"`
	LastError         string          `json:"lastError,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

type BackupEngineSnapshot struct {
	ID             string          `json:"id"`
	RepoID         string          `json:"repoId"`
	SnapshotID     string          `json:"snapshotId"`
	Paths          json.RawMessage `json:"paths"`
	Hostname       string          `json:"hostname"`
	Timestamp      *time.Time      `json:"timestamp,omitempty"`
	Parent         string          `json:"parent,omitempty"`
	Tree           string          `json:"tree,omitempty"`
	DataFiles      int64           `json:"dataFiles"`
	TotalSizeBytes int64           `json:"totalSizeBytes"`
	VerifiedAt     *time.Time      `json:"verifiedAt,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
}

type BackupEngineRestoreJob struct {
	ID          string     `json:"id"`
	RepoID      string     `json:"repoId"`
	SnapshotID  string     `json:"snapshotId"`
	TargetPath  string     `json:"targetPath"`
	Status      string     `json:"status"`
	ProgressPct int        `json:"progressPct"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

const backupEngineRepositoryColumns = `id, name, engine, location, password_ref, password_encrypted,
	encryption, prune_policy, node_id, server_id, artifact_id, initialized,
	last_snapshot_at, next_run_at, COALESCE(last_error, ''), created_at, updated_at`

func scanBackupEngineRepository(row pgx.Row) (BackupEngineRepository, error) {
	var r BackupEngineRepository
	err := row.Scan(&r.ID, &r.Name, &r.Engine, &r.Location, &r.PasswordRef, &r.PasswordEncrypted,
		&r.Encryption, &r.PrunePolicy, &r.NodeID, &r.ServerID, &r.ArtifactID, &r.Initialized,
		&r.LastSnapshotAt, &r.NextRunAt, &r.LastError, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *Store) ListBackupRepositories(ctx context.Context) ([]BackupEngineRepository, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupEngineRepositoryColumns+` FROM backup_repositories ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	repos := make([]BackupEngineRepository, 0, 8)
	for rows.Next() {
		repo, err := scanBackupEngineRepository(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, rows.Err()
}

func (s *Store) GetBackupRepository(ctx context.Context, id string) (BackupEngineRepository, error) {
	return scanBackupEngineRepository(s.db.QueryRow(ctx,
		`SELECT `+backupEngineRepositoryColumns+` FROM backup_repositories WHERE id = $1`, id))
}

// CreateBackupRepository seals the repository password through the store
// secret layer (same envelope scheme as notification channels / nodes) and
// persists both the operator-facing ref and the encrypted blob.
func (s *Store) CreateBackupRepository(ctx context.Context, r *BackupEngineRepository) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if r.Engine != "restic" && r.Engine != "kopia" {
		return fmt.Errorf("unsupported backup engine %q", r.Engine)
	}
	if len(r.PrunePolicy) == 0 {
		r.PrunePolicy = json.RawMessage(`{}`)
	}
	if r.Encryption == "" {
		r.Encryption = "aes-256"
	}
	var err error
	r.PasswordEncrypted, err = s.encryptSecret(r.PasswordRef, secretAAD("backup_repositories", r.ID, "password"))
	if err != nil {
		return fmt.Errorf("seal repository password: %w", err)
	}
	now := time.Now().UTC()
	r.CreatedAt, r.UpdatedAt = now, now
	_, err = s.db.Exec(ctx, `
		INSERT INTO backup_repositories (id, name, engine, location, password_ref, password_encrypted,
			encryption, prune_policy, node_id, server_id, artifact_id, initialized, last_error, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, r.ID, r.Name, r.Engine, r.Location, r.PasswordRef, r.PasswordEncrypted,
		r.Encryption, r.PrunePolicy, r.NodeID, r.ServerID, r.ArtifactID, r.Initialized,
		nilIfEmptyStr(r.LastError), r.CreatedAt, r.UpdatedAt)
	return err
}

func (s *Store) DeleteBackupRepository(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM backup_repositories WHERE id = $1`, id)
	return err
}

func (s *Store) MarkBackupRepositoryInitialized(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE backup_repositories SET initialized = TRUE, updated_at = now() WHERE id = $1`, id)
	return err
}

// UpdateBackupRepositoryRunState records the outcome of the latest snapshot /
// prune attempt and the next scheduled run. Empty lastError leaves the stored
// value untouched; a nil timestamp leaves its column untouched.
func (s *Store) UpdateBackupRepositoryRunState(ctx context.Context, id string, lastSnapshotAt, nextRunAt *time.Time, lastError *string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE backup_repositories
		SET last_snapshot_at = COALESCE($2, last_snapshot_at),
		    next_run_at      = COALESCE($3, next_run_at),
		    last_error       = COALESCE($4, last_error),
		    updated_at       = now()
		WHERE id = $1
	`, id, lastSnapshotAt, nextRunAt, lastError)
	return err
}

const backupEngineSnapshotColumns = `id, repo_id, snapshot_id, paths, hostname, snapshot_time,
	COALESCE(parent, ''), COALESCE(tree, ''), data_files, total_size_bytes, verified_at, created_at`

func scanBackupEngineSnapshot(row pgx.Row) (BackupEngineSnapshot, error) {
	var sn BackupEngineSnapshot
	var parent, tree *string
	err := row.Scan(&sn.ID, &sn.RepoID, &sn.SnapshotID, &sn.Paths, &sn.Hostname, &sn.Timestamp,
		&parent, &tree, &sn.DataFiles, &sn.TotalSizeBytes, &sn.VerifiedAt, &sn.CreatedAt)
	if err != nil {
		return sn, err
	}
	if parent != nil {
		sn.Parent = *parent
	}
	if tree != nil {
		sn.Tree = *tree
	}
	return sn, nil
}

func (s *Store) ListBackupSnapshots(ctx context.Context, repoID string) ([]BackupEngineSnapshot, error) {
	rows, err := s.db.Query(ctx, `SELECT `+backupEngineSnapshotColumns+` FROM backup_snapshots
		WHERE repo_id = $1 ORDER BY snapshot_time DESC NULLS LAST`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]BackupEngineSnapshot, 0, 16)
	for rows.Next() {
		sn, err := scanBackupEngineSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

// UpsertBackupSnapshot inserts a snapshot row reported by the CLI, refreshing
// metadata when the same snapshot id is re-listed for the same repository.
// verified_at is deliberately not overwritten so prior verification wins
// survive a re-list.
func (s *Store) UpsertBackupSnapshot(ctx context.Context, sn *BackupEngineSnapshot) error {
	if sn.ID == "" {
		sn.ID = uuid.NewString()
	}
	if len(sn.Paths) == 0 {
		sn.Paths = json.RawMessage(`[]`)
	}
	if sn.CreatedAt.IsZero() {
		sn.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO backup_snapshots (id, repo_id, snapshot_id, paths, hostname, snapshot_time,
			parent, tree, data_files, total_size_bytes, verified_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9, $10, $11, $12)
		ON CONFLICT (repo_id, snapshot_id) DO UPDATE SET
			paths            = EXCLUDED.paths,
			hostname         = EXCLUDED.hostname,
			snapshot_time    = EXCLUDED.snapshot_time,
			parent           = EXCLUDED.parent,
			tree             = EXCLUDED.tree,
			data_files       = EXCLUDED.data_files,
			total_size_bytes = EXCLUDED.total_size_bytes
	`, sn.ID, sn.RepoID, sn.SnapshotID, sn.Paths, sn.Hostname, sn.Timestamp,
		sn.Parent, sn.Tree, sn.DataFiles, sn.TotalSizeBytes, sn.VerifiedAt, sn.CreatedAt)
	return err
}

func (s *Store) MarkBackupSnapshotVerified(ctx context.Context, repoID, snapshotID string, at time.Time) error {
	_, err := s.db.Exec(ctx, `
		UPDATE backup_snapshots SET verified_at = $3
		WHERE repo_id = $1 AND snapshot_id = $2
	`, repoID, snapshotID, at)
	return err
}

func (s *Store) DeleteBackupSnapshot(ctx context.Context, repoID, snapshotID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM backup_snapshots WHERE repo_id = $1 AND snapshot_id = $2`, repoID, snapshotID)
	return err
}

const backupEngineRestoreJobColumns = `id, repo_id, snapshot_id, target_path, status, progress_pct,
	started_at, completed_at, COALESCE(error, ''), created_at`

func scanBackupEngineRestoreJob(row pgx.Row) (BackupEngineRestoreJob, error) {
	var j BackupEngineRestoreJob
	var errMsg *string
	err := row.Scan(&j.ID, &j.RepoID, &j.SnapshotID, &j.TargetPath, &j.Status, &j.ProgressPct,
		&j.StartedAt, &j.CompletedAt, &errMsg, &j.CreatedAt)
	if err != nil {
		return j, err
	}
	if errMsg != nil {
		j.Error = *errMsg
	}
	return j, nil
}

func (s *Store) CreateBackupRestoreJob(ctx context.Context, j *BackupEngineRestoreJob) error {
	if j.ID == "" {
		j.ID = uuid.NewString()
	}
	if j.Status == "" {
		j.Status = "pending"
	}
	j.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO backup_restore_jobs (id, repo_id, snapshot_id, target_path, status, progress_pct, started_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, j.ID, j.RepoID, j.SnapshotID, j.TargetPath, j.Status, j.ProgressPct, j.StartedAt, j.CreatedAt)
	return err
}

func (s *Store) UpdateBackupRestoreJob(ctx context.Context, id, status string, progressPct int, startedAt, completedAt *time.Time, errMsg *string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE backup_restore_jobs
		SET status       = $2,
		    progress_pct = $3,
		    started_at   = COALESCE($4, started_at),
		    completed_at = COALESCE($5, completed_at),
		    error        = COALESCE($6, error)
		WHERE id = $1
	`, id, status, progressPct, startedAt, completedAt, errMsg)
	return err
}

func (s *Store) GetBackupRestoreJob(ctx context.Context, id string) (BackupEngineRestoreJob, error) {
	return scanBackupEngineRestoreJob(s.db.QueryRow(ctx,
		`SELECT `+backupEngineRestoreJobColumns+` FROM backup_restore_jobs WHERE id = $1`, id))
}

func (s *Store) ListBackupRestoreJobs(ctx context.Context, repoID string, limit int) ([]BackupEngineRestoreJob, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	var (
		rows pgx.Rows
		err  error
	)
	if repoID == "" {
		rows, err = s.db.Query(ctx, `SELECT `+backupEngineRestoreJobColumns+` FROM backup_restore_jobs
			ORDER BY created_at DESC LIMIT $1`, limit)
	} else {
		rows, err = s.db.Query(ctx, `SELECT `+backupEngineRestoreJobColumns+` FROM backup_restore_jobs
			WHERE repo_id = $1 ORDER BY created_at DESC LIMIT $2`, repoID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]BackupEngineRestoreJob, 0, 16)
	for rows.Next() {
		j, err := scanBackupEngineRestoreJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// BackupEngineNodeTarget resolves the beacon control endpoint for a
// backup-engine node (mirrors ServerControlTarget's token sealing).
func (s *Store) BackupEngineNodeTarget(ctx context.Context, nodeID string) (ServerControlTarget, error) {
	var target ServerControlTarget
	var resolvedID, tokenID, daemonToken, daemonTokenEncrypted string
	err := s.db.QueryRow(ctx, `
		SELECT n.id::text, n.base_url,
		       COALESCE(n.daemon_token_id, ''),
		       COALESCE(n.daemon_token, ''),
		       COALESCE(n.daemon_token_encrypted, '')
		FROM nodes n
		WHERE n.id = $1
	`, nodeID).Scan(&resolvedID, &target.NodeURL, &tokenID, &daemonToken, &daemonTokenEncrypted)
	if err != nil {
		return ServerControlTarget{}, err
	}
	token, err := s.decryptSecret(daemonTokenEncrypted, daemonToken, secretAAD("nodes", resolvedID, "daemon_token"))
	if err != nil {
		return ServerControlTarget{}, err
	}
	if tokenID != "" && token != "" {
		target.NodeToken = tokenID + "." + token
	}
	target.ServerID = resolvedID
	return target, nil
}

// BackupEngineExecContainer resolves the running container id for a server so
// the backup engine can exec its CLI through the beacon's admin exec endpoint.
// It reads the most recent workload observation; an empty result means the
// caller should fall back to host-local execution.
func (s *Store) BackupEngineExecContainer(ctx context.Context, serverID string) (string, error) {
	if serverID == "" {
		return "", nil
	}
	var containerID string
	err := s.db.QueryRow(ctx, `
		SELECT container_id FROM workload_metrics
		WHERE server_id = $1 AND container_id <> ''
		ORDER BY observed_at DESC LIMIT 1
	`, serverID).Scan(&containerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return containerID, err
}

// DecryptBackupRepositoryPassword returns the sealed repository password,
// falling back to the plaintext ref for rows written before sealing.
func (s *Store) DecryptBackupRepositoryPassword(repo BackupEngineRepository) (string, error) {
	return s.decryptSecret(repo.PasswordEncrypted, repo.PasswordRef, secretAAD("backup_repositories", repo.ID, "password"))
}

func nilIfEmptyStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}
