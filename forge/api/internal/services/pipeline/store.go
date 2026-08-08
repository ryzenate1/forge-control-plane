package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the persistence layer for pipelines, runs, stage runs, logs and
// artifact metadata. Implemented directly over the shared postgres pool that
// Store.DB() exposes so the pipeline phase stays fully additive (no edits to
// existing store files).
type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// ---- pipeline definitions ----

func (s *Store) CreateDefinition(ctx context.Context, def *Definition) error {
	def.ID = uuid.NewString()
	stages, err := json.Marshal(def.Stages)
	if err != nil {
		return fmt.Errorf("marshal stages: %w", err)
	}
	trigger, err := json.Marshal(def.Trigger)
	if err != nil {
		return fmt.Errorf("marshal trigger: %w", err)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO pipeline_defs (id, name, description, categories, stages, trigger, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		def.ID, def.Name, def.Description, def.Categories, stages, trigger, def.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert pipeline def: %w", err)
	}
	return nil
}

func (s *Store) GetDefinition(ctx context.Context, id string) (*Definition, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, name, description, categories, stages, trigger, created_by, created_at, updated_at
		FROM pipeline_defs WHERE id = $1`, id)
	def, err := scanDefinition(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return def, nil
}

func (s *Store) ListDefinitions(ctx context.Context) ([]*Definition, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, description, categories, stages, trigger, created_by, created_at, updated_at
		FROM pipeline_defs ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list pipeline defs: %w", err)
	}
	defer rows.Close()
	defs := []*Definition{}
	for rows.Next() {
		def, err := scanDefinition(rows)
		if err != nil {
			return nil, err
		}
		defs = append(defs, def)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pipeline defs: %w", err)
	}
	return defs, nil
}

func (s *Store) UpdateDefinition(ctx context.Context, id string, def *Definition) error {
	stages, err := json.Marshal(def.Stages)
	if err != nil {
		return fmt.Errorf("marshal stages: %w", err)
	}
	trigger, err := json.Marshal(def.Trigger)
	if err != nil {
		return fmt.Errorf("marshal trigger: %w", err)
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE pipeline_defs
		SET name = $2, description = $3, categories = $4, stages = $5, trigger = $6, updated_at = now()
		WHERE id = $1`,
		id, def.Name, def.Description, def.Categories, stages, trigger)
	if err != nil {
		return fmt.Errorf("update pipeline def: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteDefinition(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM pipeline_defs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete pipeline def: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateRun creates a run plus its materialised stage rows in one
// transaction. Stages come from the frozen snapshot of the definition.
func (s *Store) CreateRun(ctx context.Context, run *Run, stages []StageRun) error {
	run.ID = uuid.NewString()
	snapshot, err := json.Marshal(stages)
	if err != nil {
		return fmt.Errorf("marshal stage snapshot: %w", err)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin run tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var retryOf any
	if run.RetryOf != nil {
		retryOf = *run.RetryOf
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO pipeline_runs
			(id, pipeline_id, trigger, status, stage_snapshot, retry_of, retry_count, requested_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		run.ID, run.PipelineID, run.Trigger, StatusQueued, snapshot, retryOf, run.RetryCount, run.RequestedBy)
	if err != nil {
		return fmt.Errorf("insert pipeline run: %w", err)
	}
	for i := range stages {
		stages[i].ID = uuid.NewString()
		stages[i].RunID = run.ID
		stages[i].PipelineID = run.PipelineID
		cfg, err := json.Marshal(stages[i].Config)
		if err != nil {
			return fmt.Errorf("marshal stage config: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO pipeline_stage_runs
				(id, run_id, pipeline_id, position, name, action, config, status,
				 continue_on_failure, timeout_sec, retry_max, retry_backoff_ms, retry_max_sleep_ms)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			stages[i].ID, run.ID, run.PipelineID, stages[i].Position, stages[i].Name,
			stages[i].Action, cfg, StageStatusQueued,
			stages[i].ContinueOnFailure, stages[i].TimeoutSec,
			stages[i].RetryPolicy.MaxRetries, stages[i].RetryPolicy.BackoffMs,
			stages[i].RetryPolicy.MaxSleepMs)
		if err != nil {
			return fmt.Errorf("insert stage run: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit run tx: %w", err)
	}
	return nil
}

func (s *Store) GetRun(ctx context.Context, id string) (*Run, error) {
	row := s.db.QueryRow(ctx, `
		SELECT r.id, r.pipeline_id, d.name, r.trigger, r.status, r.progress_pct,
		       r.current_stage, r.error, r.retry_of, r.retry_count, r.cancel_requested,
		       r.requested_by, r.queued_at, r.started_at, r.finished_at, r.created_at, r.updated_at
		FROM pipeline_runs r
		LEFT JOIN pipeline_defs d ON d.id = r.pipeline_id
		WHERE r.id = $1`, id)
	run, err := scanRun(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	stages, err := s.ListStageRuns(ctx, id)
	if err != nil {
		return nil, err
	}
	run.Stages = stages
	return run, nil
}

func (s *Store) ListRuns(ctx context.Context, pipelineID, status string, limit, offset int) ([]*Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := `
		SELECT r.id, r.pipeline_id, d.name, r.trigger, r.status, r.progress_pct,
		       r.current_stage, r.error, r.retry_of, r.retry_count, r.cancel_requested,
		       r.requested_by, r.queued_at, r.started_at, r.finished_at, r.created_at, r.updated_at
		FROM pipeline_runs r
		LEFT JOIN pipeline_defs d ON d.id = r.pipeline_id`
	args := []any{}
	clauses := []string{}
	if pipelineID != "" {
		args = append(args, pipelineID)
		clauses = append(clauses, fmt.Sprintf("r.pipeline_id = $%d", len(args)))
	}
	if status != "" {
		args = append(args, status)
		clauses = append(clauses, fmt.Sprintf("r.status = $%d", len(args)))
	}
	if len(clauses) > 0 {
		q += " WHERE " + joinClauses(clauses)
	}
	args = append(args, limit, offset)
	q += fmt.Sprintf(" ORDER BY r.created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list pipeline runs: %w", err)
	}
	defer rows.Close()
	runs := []*Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pipeline runs: %w", err)
	}
	return runs, nil
}

// ClaimQueuedRuns atomically takes up to n queued runs (SKIP LOCKED so
// multiple API instances never double-execute) and moves them to running.
func (s *Store) ClaimQueuedRuns(ctx context.Context, n int) ([]*Run, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id FROM pipeline_runs
		WHERE status = $1
		ORDER BY queued_at, id
		LIMIT $2
		FOR UPDATE SKIP LOCKED`, StatusQueued, n)
	if err != nil {
		return nil, fmt.Errorf("claim queued runs: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	claimed := []*Run{}
	for _, id := range ids {
		tag, err := s.db.Exec(ctx, `
			UPDATE pipeline_runs SET status = $2, started_at = now(), updated_at = now()
			WHERE id = $1 AND status = $3`, id, StatusRunning, StatusQueued)
		if err != nil {
			return nil, fmt.Errorf("claim run %s: %w", id, err)
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		run, err := s.GetRun(ctx, id)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, run)
	}
	return claimed, nil
}

func (s *Store) MarkRunProgress(ctx context.Context, runID, status, currentStage string, pct int, errMsg string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE pipeline_runs
		SET status = $2, current_stage = $3, progress_pct = $4,
		    error = CASE WHEN $5 = '' THEN error ELSE $5 END,
		    updated_at = now()
		WHERE id = $1`, runID, status, currentStage, pct, errMsg)
	if err != nil {
		return fmt.Errorf("mark run progress: %w", err)
	}
	return nil
}

func (s *Store) SetRunFinished(ctx context.Context, runID, status, errMsg string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE pipeline_runs
		SET status = $2, error = CASE WHEN $3 = '' THEN error ELSE $3 END,
		    finished_at = now(), updated_at = now()
		WHERE id = $1 AND finished_at IS NULL`, runID, status, errMsg)
	if err != nil {
		return fmt.Errorf("finish run: %w", err)
	}
	if status == StatusFailed || status == StatusCancelled {
		_, _ = s.db.Exec(ctx, `
			UPDATE pipeline_stage_runs SET status = $2, finished_at = now(), updated_at = now()
			WHERE run_id = $1 AND status IN ($3, $4)`, runID, StageStatusCancelled, StageStatusQueued, StageStatusRunning)
	}
	return nil
}

func (s *Store) SetRunCancelRequested(ctx context.Context, runID string) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE pipeline_runs SET cancel_requested = TRUE, updated_at = now()
		WHERE id = $1 AND status IN ($2, $3, $4)`,
		runID, StatusQueued, StatusRunning, StatusAwaitApproval)
	if err != nil {
		return false, fmt.Errorf("request run cancel: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) LatestRunForPipeline(ctx context.Context, pipelineID string) (*Run, error) {
	row := s.db.QueryRow(ctx, `
		SELECT r.id, r.pipeline_id, d.name, r.trigger, r.status, r.progress_pct,
		       r.current_stage, r.error, r.retry_of, r.retry_count, r.cancel_requested,
		       r.requested_by, r.queued_at, r.started_at, r.finished_at, r.created_at, r.updated_at
		FROM pipeline_runs r
		LEFT JOIN pipeline_defs d ON d.id = r.pipeline_id
		WHERE r.pipeline_id = $1
		ORDER BY r.created_at DESC LIMIT 1`, pipelineID)
	run, err := scanRun(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return run, nil
}

// stage runs

func (s *Store) ListStageRuns(ctx context.Context, runID string) ([]StageRun, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, run_id, pipeline_id, position, name, action, config, status,
		       attempts, continue_on_failure, timeout_sec, retry_max, retry_backoff_ms, retry_max_sleep_ms,
		       error, started_at, finished_at, created_at, updated_at
		FROM pipeline_stage_runs WHERE run_id = $1 ORDER BY position`, runID)
	if err != nil {
		return nil, fmt.Errorf("list stage runs: %w", err)
	}
	defer rows.Close()
	stages := []StageRun{}
	for rows.Next() {
		st, err := scanStageRun(rows)
		if err != nil {
			return nil, err
		}
		stages = append(stages, *st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stage runs: %w", err)
	}
	return stages, nil
}

func (s *Store) GetStageRun(ctx context.Context, stageID string) (*StageRun, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, run_id, pipeline_id, position, name, action, config, status,
		       attempts, continue_on_failure, timeout_sec, retry_max, retry_backoff_ms, retry_max_sleep_ms,
		       error, started_at, finished_at, created_at, updated_at
		FROM pipeline_stage_runs WHERE id = $1`, stageID)
	st, err := scanStageRun(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return st, nil
}

func (s *Store) SetStageStatus(ctx context.Context, stageID, status, errMsg string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE pipeline_stage_runs
		SET status = $2, error = CASE WHEN $3 = '' THEN error ELSE $3 END,
		    finished_at = CASE WHEN $2 IN ('succeeded', 'failed', 'cancelled', 'skipped')
		              THEN now() ELSE finished_at END,
		    updated_at = now()
		WHERE id = $1`, stageID, status, errMsg)
	if err != nil {
		return fmt.Errorf("set stage status: %w", err)
	}
	return nil
}

func (s *Store) StartStage(ctx context.Context, stageID string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE pipeline_stage_runs
		SET status = $2, attempts = attempts + 1, started_at = COALESCE(started_at, now()),
		    updated_at = now()
		WHERE id = $1`, stageID, StageStatusRunning)
	if err != nil {
		return fmt.Errorf("start stage: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ResetStageRun(ctx context.Context, runID, stageID string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE pipeline_stage_runs
		SET status = $3, error = '', updated_at = now()
		WHERE run_id = $1 AND id = $2 AND status = $4`,
		runID, stageID, StageStatusQueued, StageStatusAwaiting)
	if err != nil {
		return fmt.Errorf("reset stage: %w", err)
	}
	return nil
}

// logs

func (s *Store) AppendLog(ctx context.Context, runID, stageID, level, message string) (LogEntry, error) {
	var entry LogEntry
	err := s.db.QueryRow(ctx, `
		INSERT INTO pipeline_stage_logs (run_id, stage_id, level, message)
		VALUES ($1, $2, $3, $4)
		RETURNING id, run_id, stage_id, level, message, created_at`,
		runID, stageID, level, message).Scan(&entry.ID, &entry.RunID, &entry.StageID, &entry.Level, &entry.Message, &entry.CreatedAt)
	if err != nil {
		return LogEntry{}, fmt.Errorf("append log: %w", err)
	}
	return entry, nil
}

func (s *Store) ListLogsAfter(ctx context.Context, runID string, afterID int64, limit int) ([]LogEntry, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, run_id, stage_id, level, message, created_at
		FROM pipeline_stage_logs
		WHERE run_id = $1 AND id > $2
		ORDER BY id LIMIT $3`, runID, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list logs: %w", err)
	}
	defer rows.Close()
	entries := []LogEntry{}
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.RunID, &e.StageID, &e.Level, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) CurrentLogSeq(ctx context.Context, runID string) (int64, error) {
	var seq int64
	err := s.db.QueryRow(ctx, `SELECT COALESCE(MAX(id), 0) FROM pipeline_stage_logs WHERE run_id = $1`, runID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("current log seq: %w", err)
	}
	return seq, nil
}

// artifacts

func (s *Store) CreateArtifact(ctx context.Context, art *Artifact) error {
	art.ID = uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO pipeline_artifacts (id, run_id, stage_id, name, relative_path, size_bytes, content_type, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		art.ID, art.RunID, art.StageID, art.Name, art.RelativePath, art.SizeBytes, art.ContentType, art.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert artifact: %w", err)
	}
	return nil
}

func (s *Store) ListArtifacts(ctx context.Context, runID string) ([]Artifact, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, run_id, stage_id, name, relative_path, size_bytes, content_type, created_at
		FROM pipeline_artifacts WHERE run_id = $1 ORDER BY created_at`, runID)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	defer rows.Close()
	arts := []Artifact{}
	for rows.Next() {
		var a Artifact
		if err := rows.Scan(&a.ID, &a.RunID, &a.StageID, &a.Name, &a.RelativePath, &a.SizeBytes, &a.ContentType, &a.CreatedAt); err != nil {
			return nil, err
		}
		arts = append(arts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return arts, nil
}

func (s *Store) GetArtifact(ctx context.Context, artID string) (*Artifact, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, run_id, stage_id, name, relative_path, size_bytes, content_type, created_at
		FROM pipeline_artifacts WHERE id = $1`, artID)
	var a Artifact
	if err := row.Scan(&a.ID, &a.RunID, &a.StageID, &a.Name, &a.RelativePath, &a.SizeBytes, &a.ContentType, &a.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get artifact: %w", err)
	}
	return &a, nil
}

func (s *Store) DeleteArtifact(ctx context.Context, artID string) (*Artifact, error) {
	art, err := s.GetArtifact(ctx, artID)
	if err != nil {
		return nil, err
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM pipeline_artifacts WHERE id = $1`, artID)
	if err != nil {
		return nil, fmt.Errorf("delete artifact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return art, nil
}

// scan helpers

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDefinition(r rowScanner) (*Definition, error) {
	var def Definition
	var categories []string
	var stagesJSON, triggerJSON []byte
	err := r.Scan(&def.ID, &def.Name, &def.Description, &categories, &stagesJSON, &triggerJSON, &def.CreatedBy, &def.CreatedAt, &def.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan pipeline def: %w", err)
	}
	if err := json.Unmarshal(stagesJSON, &def.Stages); err != nil {
		return nil, fmt.Errorf("unmarshal stages: %w", err)
	}
	if err := json.Unmarshal(triggerJSON, &def.Trigger); err != nil {
		return nil, fmt.Errorf("unmarshal trigger: %w", err)
	}
	def.Categories = categories
	return &def, nil
}

func scanRun(r rowScanner) (*Run, error) {
	var run Run
	if err := r.Scan(&run.ID, &run.PipelineID, &run.PipelineName, &run.Trigger, &run.Status,
		&run.ProgressPct, &run.CurrentStage, &run.Error, &run.RetryOf, &run.RetryCount,
		&run.CancelRequested, &run.RequestedBy, &run.QueuedAt, &run.StartedAt, &run.FinishedAt,
		&run.CreatedAt, &run.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan pipeline run: %w", err)
	}
	return &run, nil
}

func scanStageRun(r rowScanner) (*StageRun, error) {
	var st StageRun
	var cfg []byte
	err := r.Scan(&st.ID, &st.RunID, &st.PipelineID, &st.Position, &st.Name, &st.Action,
		&cfg, &st.Status, &st.Attempts, &st.ContinueOnFailure, &st.TimeoutSec,
		&st.RetryPolicy.MaxRetries, &st.RetryPolicy.BackoffMs, &st.RetryPolicy.MaxSleepMs,
		&st.Error, &st.StartedAt, &st.FinishedAt, &st.CreatedAt, &st.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan stage run: %w", err)
	}
	st.Config = map[string]any{}
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &st.Config); err != nil {
			return nil, fmt.Errorf("unmarshal stage config: %w", err)
		}
	}
	return &st, nil
}

func joinClauses(clauses []string) string {
	out := ""
	for i, c := range clauses {
		if i > 0 {
			out += " AND "
		}
		out += c
	}
	return out
}