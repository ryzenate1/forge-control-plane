package store

import (
	"context"
	"time"
)

type BuildRecord struct {
	ID           string     `json:"id"`
	SourceID     string     `json:"sourceId"`
	BuilderType  string     `json:"builderType"`
	Status       string     `json:"status"`
	ImageRef     string     `json:"imageRef,omitempty"`
	BuildLog     string     `json:"buildLog,omitempty"`
	StartedAt    time.Time  `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	ExitCode     *int       `json:"exitCode,omitempty"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	PID          *int       `json:"pid,omitempty"`
}

func (s *Store) CreateBuild(ctx context.Context, record *BuildRecord) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO builds (id, source_id, builder_type, status, image_ref, build_log, started_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, record.ID, record.SourceID, record.BuilderType, record.Status, record.ImageRef, record.BuildLog, record.StartedAt)
	return err
}

func (s *Store) UpdateBuild(ctx context.Context, record *BuildRecord) error {
	_, err := s.db.Exec(ctx, `
		UPDATE builds SET
			status = $1, image_ref = $2, build_log = $3,
			finished_at = $4, exit_code = $5, error_message = $6,
			updated_at = NOW()
		WHERE id = $7
	`, record.Status, record.ImageRef, record.BuildLog, record.FinishedAt, record.ExitCode, record.ErrorMessage, record.ID)
	return err
}

func (s *Store) GetBuild(ctx context.Context, id string) (*BuildRecord, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, source_id, builder_type, status, image_ref, build_log, started_at, finished_at, exit_code, error_message
		FROM builds WHERE id = $1
	`, id)
	var b BuildRecord
	var imageRef, buildLog, errorMessage *string
	var finishedAt *time.Time
	var exitCode *int
	err := row.Scan(&b.ID, &b.SourceID, &b.BuilderType, &b.Status, &imageRef, &buildLog, &b.StartedAt, &finishedAt, &exitCode, &errorMessage)
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
	return &b, nil
}

func (s *Store) ListBuilds(ctx context.Context, sourceID string) ([]*BuildRecord, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, source_id, builder_type, status, image_ref, build_log, started_at, finished_at, exit_code, error_message
		FROM builds WHERE source_id = $1 ORDER BY started_at DESC LIMIT 100
	`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var builds []*BuildRecord
	for rows.Next() {
		var b BuildRecord
		var imageRef, buildLog, errorMessage *string
		var finishedAt *time.Time
		var exitCode *int
		if err := rows.Scan(&b.ID, &b.SourceID, &b.BuilderType, &b.Status, &imageRef, &buildLog, &b.StartedAt, &finishedAt, &exitCode, &errorMessage); err != nil {
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
