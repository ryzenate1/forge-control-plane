package store

import (
	"context"
	"time"
)

type BuildRecord struct {
	ID                string     `json:"id"`
	SourceID          string     `json:"sourceId"`
	BuilderType       string     `json:"builderType"`
	Status            string     `json:"status"`
	BuildStage        string     `json:"buildStage"`
	ImageRef          string     `json:"imageRef,omitempty"`
	BuildLog          string     `json:"buildLog,omitempty"`
	StartedAt         time.Time  `json:"startedAt"`
	FinishedAt        *time.Time `json:"finishedAt,omitempty"`
	ExitCode          *int       `json:"exitCode,omitempty"`
	ErrorMessage      string     `json:"errorMessage,omitempty"`
	PID               *int       `json:"pid,omitempty"`

	// Extended build pipeline fields
	NodeID            string     `json:"nodeId,omitempty"`
	WorkspaceID       string     `json:"workspaceId,omitempty"`
	Registry          string     `json:"registry,omitempty"`
	CacheFrom         []string   `json:"cacheFrom,omitempty"`
	CacheTo           []string   `json:"cacheTo,omitempty"`
	Platform          string     `json:"platform,omitempty"`
	CommitSHA         string     `json:"commitSha,omitempty"`
	CommitRef         string     `json:"commitRef,omitempty"`
	Digest            string     `json:"digest,omitempty"`
	RetryOf           string     `json:"retryOf,omitempty"`
	RetryAttempt      int        `json:"retryAttempt,omitempty"`
	TimedOut          bool       `json:"timedOut,omitempty"`
	BuildTimeout      int        `json:"buildTimeoutSecs,omitempty"`
	CredMasked        bool       `json:"credentialsMasked,omitempty"`
	IdempotencyKey    string     `json:"idempotencyKey,omitempty"`
	BeaconBuildID     string     `json:"beaconBuildId,omitempty"`
}

func (s *Store) CreateBuild(ctx context.Context, record *BuildRecord) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO builds (
			id, source_id, builder_type, status, build_stage, image_ref, build_log, started_at,
			node_id, workspace_id, registry, cache_from, cache_to, platform,
			commit_sha, commit_ref, retry_of, retry_attempt,
			build_timeout_secs, credentials_masked, idempotency_key, beacon_build_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
	`, record.ID, record.SourceID, record.BuilderType, record.Status, record.BuildStage,
		record.ImageRef, record.BuildLog, record.StartedAt,
		record.NodeID, record.WorkspaceID, record.Registry, record.CacheFrom, record.CacheTo,
		record.Platform, record.CommitSHA, record.CommitRef,
		record.RetryOf, record.RetryAttempt, record.BuildTimeout, record.CredMasked,
		record.IdempotencyKey, record.BeaconBuildID)
	return err
}

func (s *Store) UpdateBuild(ctx context.Context, record *BuildRecord) error {
	_, err := s.db.Exec(ctx, `
		UPDATE builds SET
			status = $1, build_stage = $2, image_ref = $3, build_log = $4,
			finished_at = $5, exit_code = $6, error_message = $7,
			node_id = $8, workspace_id = $9, registry = $10, cache_from = $11, cache_to = $12,
			platform = $13, commit_sha = $14, commit_ref = $15,
			digest = $16, retry_of = $17, retry_attempt = $18,
			timed_out = $19, build_timeout_secs = $20, credentials_masked = $21,
			idempotency_key = $22, beacon_build_id = $23,
			updated_at = NOW()
		WHERE id = $24
	`, record.Status, record.BuildStage, record.ImageRef, record.BuildLog,
		record.FinishedAt, record.ExitCode, record.ErrorMessage,
		record.NodeID, record.WorkspaceID, record.Registry, record.CacheFrom, record.CacheTo,
		record.Platform, record.CommitSHA, record.CommitRef,
		record.Digest, record.RetryOf, record.RetryAttempt,
		record.TimedOut, record.BuildTimeout, record.CredMasked,
		record.IdempotencyKey, record.BeaconBuildID,
		record.ID)
	return err
}

func (s *Store) GetBuild(ctx context.Context, id string) (*BuildRecord, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, source_id, builder_type, status, build_stage, image_ref, build_log,
			started_at, finished_at, exit_code, error_message,
			node_id, workspace_id, registry, cache_from, cache_to, platform,
			commit_sha, commit_ref, digest, retry_of, retry_attempt,
			timed_out, build_timeout_secs, credentials_masked, idempotency_key, beacon_build_id
		FROM builds WHERE id = $1
	`, id)
	var b BuildRecord
	var imageRef, buildLog, errorMessage, nodeID, workspaceID, registry *string
	var finishedAt *time.Time
	var exitCode, retryAttempt, buildTimeout *int
	var timedOut, credMasked *bool
	var commitSHA, commitRef, digest, retryOf, buildStage, idempotencyKey, beaconBuildID *string
	err := row.Scan(&b.ID, &b.SourceID, &b.BuilderType, &b.Status,
		&buildStage, &imageRef, &buildLog, &b.StartedAt, &finishedAt, &exitCode, &errorMessage,
		&nodeID, &workspaceID, &registry, &b.CacheFrom, &b.CacheTo, &b.Platform,
		&commitSHA, &commitRef, &digest, &retryOf, &retryAttempt,
		&timedOut, &buildTimeout, &credMasked, &idempotencyKey, &beaconBuildID)
	if err != nil {
		return nil, err
	}
	if imageRef != nil {
		b.ImageRef = *imageRef
	}
	if buildLog != nil {
		b.BuildLog = *buildLog
	}
	if finishedAt != nil {
		b.FinishedAt = finishedAt
	}
	if exitCode != nil {
		b.ExitCode = exitCode
	}
	if errorMessage != nil {
		b.ErrorMessage = *errorMessage
	}
	if nodeID != nil {
		b.NodeID = *nodeID
	}
	if registry != nil {
		b.Registry = *registry
	}
	if commitSHA != nil {
		b.CommitSHA = *commitSHA
	}
	if commitRef != nil {
		b.CommitRef = *commitRef
	}
	if digest != nil {
		b.Digest = *digest
	}
	if retryOf != nil {
		b.RetryOf = *retryOf
	}
	if retryAttempt != nil {
		b.RetryAttempt = *retryAttempt
	}
	if buildTimeout != nil {
		b.BuildTimeout = *buildTimeout
	}
	if timedOut != nil {
		b.TimedOut = *timedOut
	}
	if credMasked != nil {
		b.CredMasked = *credMasked
	}
	if buildStage != nil {
		b.BuildStage = *buildStage
	}
	if workspaceID != nil {
		b.WorkspaceID = *workspaceID
	}
	if idempotencyKey != nil {
		b.IdempotencyKey = *idempotencyKey
	}
	if beaconBuildID != nil {
		b.BeaconBuildID = *beaconBuildID
	}
	return &b, nil
}

func (s *Store) ListBuilds(ctx context.Context, sourceID string) ([]*BuildRecord, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, source_id, builder_type, status, build_stage, image_ref, build_log,
			started_at, finished_at, exit_code, error_message,
			node_id, workspace_id, registry, cache_from, cache_to, platform,
			commit_sha, commit_ref, digest, retry_of, retry_attempt,
			timed_out, build_timeout_secs, credentials_masked, idempotency_key, beacon_build_id
		FROM builds WHERE source_id = $1 ORDER BY started_at DESC LIMIT 100
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var builds []*BuildRecord
	for rows.Next() {
		var b BuildRecord
		var imageRef, buildLog, errorMessage, nodeID, workspaceID, registry *string
		var finishedAt *time.Time
		var exitCode, retryAttempt, buildTimeout *int
		var timedOut, credMasked *bool
		var commitSHA, commitRef, digest, retryOf, buildStage, idempotencyKey, beaconBuildID *string
		if err := rows.Scan(&b.ID, &b.SourceID, &b.BuilderType, &b.Status,
			&buildStage, &imageRef, &buildLog, &b.StartedAt, &finishedAt, &exitCode, &errorMessage,
			&nodeID, &workspaceID, &registry, &b.CacheFrom, &b.CacheTo, &b.Platform,
			&commitSHA, &commitRef, &digest, &retryOf, &retryAttempt,
			&timedOut, &buildTimeout, &credMasked, &idempotencyKey, &beaconBuildID); err != nil {
			return nil, err
		}
		if imageRef != nil {
			b.ImageRef = *imageRef
		}
		if buildLog != nil {
			b.BuildLog = *buildLog
		}
		if finishedAt != nil {
			b.FinishedAt = finishedAt
		}
		if exitCode != nil {
			b.ExitCode = exitCode
		}
		if errorMessage != nil {
			b.ErrorMessage = *errorMessage
		}
		if nodeID != nil {
			b.NodeID = *nodeID
		}
		if registry != nil {
			b.Registry = *registry
		}
		if commitSHA != nil {
			b.CommitSHA = *commitSHA
		}
		if commitRef != nil {
			b.CommitRef = *commitRef
		}
		if digest != nil {
			b.Digest = *digest
		}
		if retryOf != nil {
			b.RetryOf = *retryOf
		}
		if retryAttempt != nil {
			b.RetryAttempt = *retryAttempt
		}
		if buildTimeout != nil {
			b.BuildTimeout = *buildTimeout
		}
		if timedOut != nil {
			b.TimedOut = *timedOut
		}
		if credMasked != nil {
			b.CredMasked = *credMasked
		}
		if buildStage != nil {
			b.BuildStage = *buildStage
		}
		if workspaceID != nil {
			b.WorkspaceID = *workspaceID
		}
		if idempotencyKey != nil {
			b.IdempotencyKey = *idempotencyKey
		}
		if beaconBuildID != nil {
			b.BeaconBuildID = *beaconBuildID
		}
		builds = append(builds, &b)
	}
	return builds, rows.Err()
}

func (s *Store) PruneBuilds(ctx context.Context, sourceID string, retention int) error {
	if retention < 1 {
		retention = 20
	}
	_, err := s.db.Exec(ctx, `
		DELETE FROM builds WHERE source_id = $1 AND status IN ('succeeded','failed','canceled','abandoned') AND id NOT IN (
			SELECT id FROM builds WHERE source_id = $1 ORDER BY started_at DESC LIMIT $2
		)
	`, sourceID, retention)
	return err
}

func (s *Store) ReapAbandonedBuilds(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		UPDATE builds SET status = 'abandoned', error_message = 'build process abandoned', finished_at = NOW(), exit_code = -1, updated_at = NOW()
		WHERE status = 'running' AND started_at < NOW() - INTERVAL '12 hours'
	`)
	return err
}

func (s *Store) GetActiveBuildByIdempotencyKey(ctx context.Context, idempotencyKey string) (*BuildRecord, error) {
	if idempotencyKey == "" {
		return nil, nil
	}
	row := s.db.QueryRow(ctx, `
		SELECT id FROM builds
		WHERE idempotency_key = $1 AND status IN ('running', 'queued', 'cloning', 'building', 'pushing')
		ORDER BY started_at DESC LIMIT 1
	`, idempotencyKey)
	var id string
	if err := row.Scan(&id); err != nil {
		return nil, nil
	}
	return s.GetBuild(ctx, id)
}

func (s *Store) ListNonTerminalBuilds(ctx context.Context) ([]*BuildRecord, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, source_id, builder_type, status, build_stage, image_ref, build_log,
			started_at, finished_at, exit_code, error_message,
			node_id, workspace_id, registry, cache_from, cache_to, platform,
			commit_sha, commit_ref, digest, retry_of, retry_attempt,
			timed_out, build_timeout_secs, credentials_masked, idempotency_key, beacon_build_id
		FROM builds WHERE status IN ('running', 'queued', 'cloning', 'building', 'pushing', 'verifying_digest')
		ORDER BY started_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var builds []*BuildRecord
	for rows.Next() {
		b, err := scanBuildRow(rows)
		if err != nil {
			return nil, err
		}
		builds = append(builds, b)
	}
	return builds, rows.Err()
}

func scanBuildRow(row interface{ Scan(dest ...interface{}) error }) (*BuildRecord, error) {
	var b BuildRecord
	var imageRef, buildLog, errorMessage, nodeID, registry, workspaceID *string
	var finishedAt *time.Time
	var exitCode, retryAttempt, buildTimeout *int
	var timedOut, credMasked *bool
	var commitSHA, commitRef, digest, retryOf, buildStage, idempotencyKey, beaconBuildID *string
	err := row.Scan(&b.ID, &b.SourceID, &b.BuilderType, &b.Status,
		&buildStage, &imageRef, &buildLog, &b.StartedAt, &finishedAt, &exitCode, &errorMessage,
		&nodeID, &workspaceID, &registry, &b.CacheFrom, &b.CacheTo, &b.Platform,
		&commitSHA, &commitRef, &digest, &retryOf, &retryAttempt,
		&timedOut, &buildTimeout, &credMasked, &idempotencyKey, &beaconBuildID)
	if err != nil {
		return nil, err
	}
	if imageRef != nil {
		b.ImageRef = *imageRef
	}
	if buildLog != nil {
		b.BuildLog = *buildLog
	}
	if finishedAt != nil {
		b.FinishedAt = finishedAt
	}
	if exitCode != nil {
		b.ExitCode = exitCode
	}
	if errorMessage != nil {
		b.ErrorMessage = *errorMessage
	}
	if nodeID != nil {
		b.NodeID = *nodeID
	}
	if registry != nil {
		b.Registry = *registry
	}
	if workspaceID != nil {
		b.WorkspaceID = *workspaceID
	}
	if commitSHA != nil {
		b.CommitSHA = *commitSHA
	}
	if commitRef != nil {
		b.CommitRef = *commitRef
	}
	if digest != nil {
		b.Digest = *digest
	}
	if retryOf != nil {
		b.RetryOf = *retryOf
	}
	if retryAttempt != nil {
		b.RetryAttempt = *retryAttempt
	}
	if buildTimeout != nil {
		b.BuildTimeout = *buildTimeout
	}
	if timedOut != nil {
		b.TimedOut = *timedOut
	}
	if credMasked != nil {
		b.CredMasked = *credMasked
	}
	if buildStage != nil {
		b.BuildStage = *buildStage
	}
	if idempotencyKey != nil {
		b.IdempotencyKey = *idempotencyKey
	}
	if beaconBuildID != nil {
		b.BeaconBuildID = *beaconBuildID
	}
	return &b, nil
}
