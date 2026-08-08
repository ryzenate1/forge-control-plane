package queuepgx

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gamepanel/forge/queue"
)

//go:embed migration/*/*.sql
var migrationFS embed.FS

type Driver struct {
	dbPool *pgxpool.Pool
}

func New(dbPool *pgxpool.Pool) *Driver {
	return &Driver{dbPool: dbPool}
}

func (d *Driver) GetExecutor() queue.Executor {
	return &Executor{dbPool: d.dbPool}
}

func (d *Driver) GetListener() queue.Listener {
	return nil
}

func (d *Driver) GetMigrationFS(line string) fs.FS {
	if line == "main" {
		return migrationFS
	}
	panic("migration line does not exist: " + line)
}

func (d *Driver) GetMigrationLines() []string {
	return []string{"main"}
}

func (d *Driver) SupportsListener() bool {
	return false
}

func (d *Driver) SupportsListenNotify() bool {
	return false
}

func (d *Driver) TimePrecision() time.Duration {
	return time.Microsecond
}

func (d *Driver) UnwrapExecutor(tx pgx.Tx) queue.ExecutorTx {
	return &ExecutorTx{
		Executor: Executor{dbPool: d.dbPool, tx: &tx},
		tx:       tx,
	}
}

type Executor struct {
	dbPool *pgxpool.Pool
	tx     *pgx.Tx
}

func (e *Executor) getDB() dbtx {
	if e.tx != nil {
		return *e.tx
	}
	return e.dbPool
}

func (e *Executor) getSchemaPrefix(schema string) string {
	if schema == "" {
		return ""
	}
	return safeIdentifier(schema) + "."
}

func (e *Executor) Begin(ctx context.Context) (queue.ExecutorTx, error) {
	tx, err := e.dbPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &ExecutorTx{
		Executor: Executor{dbPool: e.dbPool, tx: &tx},
		tx:       tx,
	}, nil
}

func (e *Executor) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := e.getDB().Exec(ctx, sql, args...)
	return interpretError(err)
}

func (e *Executor) JobCancel(ctx context.Context, params *queue.JobCancelParams) (*queue.JobRow, error) {
	return queryJobCancel(ctx, e.getDB(), params.Schema, params)
}

func (e *Executor) JobGetAvailable(ctx context.Context, params *queue.JobGetAvailableParams) ([]*queue.JobRow, error) {
	return queryJobGetAvailable(ctx, e.getDB(), params.Schema, params)
}

func (e *Executor) JobGetByID(ctx context.Context, params *queue.JobGetByIDParams) (*queue.JobRow, error) {
	sql := "SELECT id, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, kind, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states FROM " + e.getSchemaPrefix(params.Schema) + "river_job WHERE id = $1"
	row := e.getDB().QueryRow(ctx, sql, params.ID)
	job, err := scanJobRow(row)
	if err != nil {
		return nil, interpretError(err)
	}
	return job, nil
}

func (e *Executor) JobGetStuck(ctx context.Context, params *queue.JobGetStuckParams) ([]*queue.JobRow, error) {
	sql := "SELECT id, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, kind, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states FROM " +
		e.getSchemaPrefix(params.Schema) +
		"river_job WHERE state = 'running' AND attempted_at <= $1 ORDER BY attempted_at LIMIT $2 FOR UPDATE SKIP LOCKED"
	rows, err := e.getDB().Query(ctx, sql, params.AttemptedAtBefore, params.Max)
	if err != nil {
		return nil, interpretError(err)
	}
	defer rows.Close()
	return scanJobRows(rows)
}

func (e *Executor) JobInsertFull(ctx context.Context, params *queue.JobInsertFullParams) (*queue.JobRow, error) {
	return queryJobInsertFull(ctx, e.getDB(), params.Schema, params)
}

func (e *Executor) JobInsertFullMany(ctx context.Context, params *queue.JobInsertFullManyParams) ([]*queue.JobRow, error) {
	if e.tx == nil {
		tx, err := e.Begin(ctx)
		if err != nil {
			return nil, err
		}
		jobs, err := tx.JobInsertFullMany(ctx, params)
		if err != nil {
			_ = tx.Rollback(ctx)
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return jobs, nil
	}
	db := e.getDB()
	var jobs []*queue.JobRow
	for _, jobParams := range params.Jobs {
		job, err := queryJobInsertFull(ctx, db, params.Schema, jobParams)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (e *Executor) JobList(ctx context.Context, params *queue.JobListParams) ([]*queue.JobRow, error) {
	whereClause := strings.TrimSpace(params.WhereClause)
	if whereClause != "" && whereClause != "true" {
		return nil, errors.New("custom SQL where clauses are not supported")
	}
	whereClause = "true"
	orderByClause := strings.TrimSpace(params.OrderByClause)
	if orderByClause == "" {
		orderByClause = "id"
	}
	switch orderByClause {
	case "id", "id ASC", "id DESC", "created_at", "created_at ASC", "created_at DESC", "scheduled_at", "scheduled_at ASC", "scheduled_at DESC":
	default:
		return nil, errors.New("unsupported job order")
	}
	if len(params.NamedArgs) != 0 {
		return nil, errors.New("named SQL arguments are not supported")
	}

	sql := "SELECT id, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, kind, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states FROM " + e.getSchemaPrefix(params.Schema) + "river_job WHERE " + whereClause + " ORDER BY " + orderByClause + " LIMIT $1::int"

	rows, err := e.getDB().Query(ctx, sql, params.Max)
	if err != nil {
		return nil, interpretError(err)
	}
	defer rows.Close()
	return scanJobRows(rows)
}

func (e *Executor) JobDeleteMany(ctx context.Context, params *queue.JobDeleteManyParams) ([]*queue.JobRow, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)
	whereClause := strings.TrimSpace(params.WhereClause)
	if whereClause != "" && whereClause != "true" {
		return nil, errors.New("custom SQL where clauses are not supported")
	}
	if len(params.NamedArgs) != 0 || strings.TrimSpace(params.OrderByClause) != "" {
		return nil, errors.New("custom SQL arguments and ordering are not supported")
	}
	sql := `WITH selected AS (
		SELECT id FROM ` + schemaPrefix + `river_job ORDER BY id LIMIT $1::int
	)
	DELETE FROM ` + schemaPrefix + `river_job
	WHERE id IN (SELECT id FROM selected)
	RETURNING id, kind, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states`
	rows, err := e.getDB().Query(ctx, sql, params.Max)
	if err != nil {
		return nil, interpretError(err)
	}
	defer rows.Close()
	return scanJobRows(rows)
}

func (e *Executor) JobGetByIDMany(ctx context.Context, params *queue.JobGetByIDManyParams) ([]*queue.JobRow, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)
	sql := `SELECT id, kind, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states FROM ` + schemaPrefix + `river_job WHERE id = ANY($1::bigint[])`
	rows, err := e.getDB().Query(ctx, sql, params.IDs)
	if err != nil {
		return nil, interpretError(err)
	}
	defer rows.Close()
	return scanJobRows(rows)
}

func (e *Executor) JobRescueMany(ctx context.Context, params *queue.JobRescueManyParams) error {
	schemaPrefix := e.getSchemaPrefix(params.Schema)
	sql := `UPDATE ` + schemaPrefix + `river_job
SET
    errors = array_append(errors, updated_job.error),
    finalized_at = updated_job.finalized_at,
    scheduled_at = updated_job.scheduled_at,
    state = updated_job.state
FROM (
    SELECT
        unnest($1::bigint[]) AS id,
        unnest($2::jsonb[]) AS error,
        nullif(unnest($3::timestamptz[]), '0001-01-01 00:00:00 +0000') AS finalized_at,
        unnest($4::timestamptz[]) AS scheduled_at,
        unnest($5::text[])::` + schemaPrefix + `river_job_state AS state
) AS updated_job
WHERE ` + schemaPrefix + `river_job.id = updated_job.id`

	var finalizedAts []time.Time
	for _, ft := range params.FinalizedAt {
		if ft != nil {
			finalizedAts = append(finalizedAts, *ft)
		} else {
			finalizedAts = append(finalizedAts, time.Time{})
		}
	}

	states := params.State
	if states == nil {
		states = []string{}
	}

	_, err := e.getDB().Exec(ctx, sql,
		params.ID,
		params.Error,
		finalizedAts,
		params.ScheduledAt,
		states,
	)
	return interpretError(err)
}

func (e *Executor) JobRetry(ctx context.Context, params *queue.JobRetryParams) (*queue.JobRow, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)

	sql := `WITH job_to_update AS (
    SELECT id
    FROM ` + schemaPrefix + `river_job
    WHERE id = $1
    FOR UPDATE
),
updated_job AS (
    UPDATE ` + schemaPrefix + `river_job
    SET
        state = 'available',
        max_attempts = CASE WHEN attempt = max_attempts THEN max_attempts + 1 ELSE max_attempts END,
        finalized_at = NULL,
        scheduled_at = coalesce($2::timestamptz, now())
    FROM job_to_update
    WHERE ` + schemaPrefix + `river_job.id = job_to_update.id
        AND ` + schemaPrefix + `river_job.state != 'running'
        AND NOT (
            ` + schemaPrefix + `river_job.state = 'available'
            AND ` + schemaPrefix + `river_job.scheduled_at < coalesce($2::timestamptz, now())
        )
    RETURNING ` + schemaPrefix + `river_job.id, ` + schemaPrefix + `river_job.args, ` + schemaPrefix + `river_job.attempt, ` + schemaPrefix + `river_job.attempted_at, ` + schemaPrefix + `river_job.attempted_by, ` + schemaPrefix + `river_job.created_at, ` + schemaPrefix + `river_job.errors, ` + schemaPrefix + `river_job.finalized_at, ` + schemaPrefix + `river_job.kind, ` + schemaPrefix + `river_job.max_attempts, ` + schemaPrefix + `river_job.metadata, ` + schemaPrefix + `river_job.priority, ` + schemaPrefix + `river_job.queue, ` + schemaPrefix + `river_job.state, ` + schemaPrefix + `river_job.scheduled_at, ` + schemaPrefix + `river_job.tags, ` + schemaPrefix + `river_job.unique_key, ` + schemaPrefix + `river_job.unique_states
)
SELECT id, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, kind, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states
FROM ` + schemaPrefix + `river_job
WHERE id = $1
    AND id NOT IN (SELECT id FROM updated_job)
UNION
SELECT id, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, kind, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states
FROM updated_job`

	row := e.getDB().QueryRow(ctx, sql, params.ID, params.Now)
	job, err := scanJobRow(row)
	if err != nil {
		return nil, interpretError(err)
	}
	return job, nil
}

func (e *Executor) JobSchedule(ctx context.Context, params *queue.JobScheduleParams) ([]*queue.JobScheduleResult, error) {
	return queryJobSchedule(ctx, e.getDB(), params.Schema, params)
}

func (e *Executor) JobSetStateIfRunningMany(ctx context.Context, params *queue.JobSetStateIfRunningManyParams) ([]*queue.JobRow, error) {
	return queryJobSetStateIfRunningMany(ctx, e.getDB(), params.Schema, params)
}

func (e *Executor) JobUpdate(ctx context.Context, params *queue.JobUpdateParams) (*queue.JobRow, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)
	metadata := params.Metadata
	if metadata == nil {
		metadata = []byte("{}")
	}

	sql := `WITH locked_job AS (
    SELECT id
    FROM ` + schemaPrefix + `river_job
    WHERE id = $1
    FOR UPDATE
)
UPDATE ` + schemaPrefix + `river_job
SET metadata = CASE WHEN $2::boolean THEN metadata || $3::jsonb ELSE metadata END
FROM locked_job
WHERE ` + schemaPrefix + `river_job.id = locked_job.id
RETURNING ` + schemaPrefix + `river_job.id, ` + schemaPrefix + `river_job.args, ` + schemaPrefix + `river_job.attempt, ` + schemaPrefix + `river_job.attempted_at, ` + schemaPrefix + `river_job.attempted_by, ` + schemaPrefix + `river_job.created_at, ` + schemaPrefix + `river_job.errors, ` + schemaPrefix + `river_job.finalized_at, ` + schemaPrefix + `river_job.kind, ` + schemaPrefix + `river_job.max_attempts, ` + schemaPrefix + `river_job.metadata, ` + schemaPrefix + `river_job.priority, ` + schemaPrefix + `river_job.queue, ` + schemaPrefix + `river_job.state, ` + schemaPrefix + `river_job.scheduled_at, ` + schemaPrefix + `river_job.tags, ` + schemaPrefix + `river_job.unique_key, ` + schemaPrefix + `river_job.unique_states`

	// The forge API's JobUpdateParams doesn't have MetadataDoMerge, so just merge
	row := e.getDB().QueryRow(ctx, sql, params.ID, true, metadata)
	job, err := scanJobRow(row)
	if err != nil {
		return nil, interpretError(err)
	}
	return job, nil
}

func (e *Executor) JobDelete(ctx context.Context, params *queue.JobDeleteParams) (*queue.JobRow, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)

	sql := `WITH job_to_delete AS (
    SELECT id
    FROM ` + schemaPrefix + `river_job
    WHERE id = $1
    FOR UPDATE
),
deleted_job AS (
    DELETE
    FROM ` + schemaPrefix + `river_job
    USING job_to_delete
    WHERE ` + schemaPrefix + `river_job.id = job_to_delete.id
        AND ` + schemaPrefix + `river_job.state != 'running'
    RETURNING ` + schemaPrefix + `river_job.id, ` + schemaPrefix + `river_job.args, ` + schemaPrefix + `river_job.attempt, ` + schemaPrefix + `river_job.attempted_at, ` + schemaPrefix + `river_job.attempted_by, ` + schemaPrefix + `river_job.created_at, ` + schemaPrefix + `river_job.errors, ` + schemaPrefix + `river_job.finalized_at, ` + schemaPrefix + `river_job.kind, ` + schemaPrefix + `river_job.max_attempts, ` + schemaPrefix + `river_job.metadata, ` + schemaPrefix + `river_job.priority, ` + schemaPrefix + `river_job.queue, ` + schemaPrefix + `river_job.state, ` + schemaPrefix + `river_job.scheduled_at, ` + schemaPrefix + `river_job.tags, ` + schemaPrefix + `river_job.unique_key, ` + schemaPrefix + `river_job.unique_states
)
SELECT id, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, kind, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states
FROM ` + schemaPrefix + `river_job
WHERE id = $1
    AND id NOT IN (SELECT id FROM deleted_job)
UNION
SELECT id, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, kind, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states
FROM deleted_job`

	row := e.getDB().QueryRow(ctx, sql, params.ID)
	job, err := scanJobRow(row)
	if err != nil {
		return nil, interpretError(err)
	}
	if job.State == queue.JobStateRunning {
		return nil, queue.ErrJobRunning
	}
	return job, nil
}

func (e *Executor) JobDeleteBefore(ctx context.Context, params *queue.JobDeleteBeforeParams) (int, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)

	sql := `DELETE FROM ` + schemaPrefix + `river_job
WHERE id IN (
    SELECT id
    FROM ` + schemaPrefix + `river_job
    WHERE state = $1
        AND finalized_at < $2::timestamptz
    ORDER BY id
    LIMIT $3::bigint
)`

	tag, err := e.getDB().Exec(ctx, sql, string(params.State), params.FinalizedAtHorizon, int64(params.Max))
	if err != nil {
		return 0, interpretError(err)
	}
	return int(tag.RowsAffected()), nil
}

func (e *Executor) LeaderAttemptElect(ctx context.Context, params *queue.LeaderElectParams) (*queue.Leader, error) {
	return queryLeaderAttemptElect(ctx, e.getDB(), params.Schema, params)
}

func (e *Executor) LeaderAttemptReelect(ctx context.Context, params *queue.LeaderReelectParams) (*queue.Leader, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)

	sql := `UPDATE ` + schemaPrefix + `river_leader
SET expires_at = coalesce($1::timestamptz, now()) + make_interval(secs => $2)
WHERE
    elected_at = $3::timestamptz
    AND expires_at >= coalesce($1::timestamptz, now())
    AND leader_id = $4
RETURNING elected_at, expires_at, leader_id, name`

	row := e.getDB().QueryRow(ctx, sql, params.Now, params.TTL.Seconds(), params.ElectedAt, params.LeaderID)
	var s leaderRowScan
	err := row.Scan(&s.ElectedAt, &s.ExpiresAt, &s.LeaderID, &s.Name)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, queue.ErrNotFound
		}
		return nil, interpretError(err)
	}
	return &queue.Leader{
		ElectedAt: s.ElectedAt,
		ExpiresAt: s.ExpiresAt,
		LeaderID:  s.LeaderID,
	}, nil
}

func (e *Executor) LeaderDeleteExpired(ctx context.Context, params *queue.LeaderDeleteExpiredParams) (int, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)

	sql := `DELETE FROM ` + schemaPrefix + `river_leader
WHERE expires_at < coalesce($1::timestamptz, now())`

	tag, err := e.getDB().Exec(ctx, sql, params.Now)
	if err != nil {
		return 0, interpretError(err)
	}
	return int(tag.RowsAffected()), nil
}

func (e *Executor) LeaderGetElectedLeader(ctx context.Context, params *queue.LeaderGetElectedLeaderParams) (*queue.Leader, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)

	sql := `SELECT elected_at, expires_at, leader_id, name
FROM ` + schemaPrefix + `river_leader`

	row := e.getDB().QueryRow(ctx, sql)
	var s leaderRowScan
	err := row.Scan(&s.ElectedAt, &s.ExpiresAt, &s.LeaderID, &s.Name)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, queue.ErrNotFound
		}
		return nil, interpretError(err)
	}
	return &queue.Leader{
		ElectedAt: s.ElectedAt,
		ExpiresAt: s.ExpiresAt,
		LeaderID:  s.LeaderID,
	}, nil
}

func (e *Executor) LeaderResign(ctx context.Context, params *queue.LeaderResignParams) (bool, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)

	sql := `WITH currently_held_leaders AS (
    SELECT elected_at, expires_at, leader_id, name
    FROM ` + schemaPrefix + `river_leader
    WHERE
        elected_at = $1::timestamptz
        AND leader_id = $2::text
    FOR UPDATE
)
DELETE FROM ` + schemaPrefix + `river_leader USING currently_held_leaders`

	tag, err := e.getDB().Exec(ctx, sql, params.ElectedAt, params.LeaderID)
	if err != nil {
		return false, interpretError(err)
	}
	return tag.RowsAffected() > 0, nil
}

func (e *Executor) NotifyMany(ctx context.Context, params *queue.NotifyManyParams) error {
	return queryNotifyMany(ctx, e.getDB(), params.Schema, params)
}

func (e *Executor) QueueCreateOrSetUpdatedAt(ctx context.Context, params *queue.QueueCreateOrSetUpdatedAtParams) error {
	return queryQueueCreateOrSetUpdatedAt(ctx, e.getDB(), params.Schema, params)
}

func (e *Executor) QueueDeleteExpired(ctx context.Context, params *queue.QueueDeleteExpiredParams) ([]string, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)

	sql := `DELETE FROM ` + schemaPrefix + `river_queue
WHERE updated_at < $1::timestamptz
RETURNING name`

	rows, err := e.getDB().Query(ctx, sql, params.UpdatedAtHorizon)
	if err != nil {
		return nil, interpretError(err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return names, nil
}

func (e *Executor) QueueGet(ctx context.Context, params *queue.QueueGetParams) (string, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)

	sql := `SELECT name
FROM ` + schemaPrefix + `river_queue
WHERE name = $1::text`

	var name string
	err := e.getDB().QueryRow(ctx, sql, params.Name).Scan(&name)
	if err != nil {
		return "", interpretError(err)
	}
	return name, nil
}

func (e *Executor) QueueList(ctx context.Context, params *queue.QueueListParams) ([]string, error) {
	schemaPrefix := e.getSchemaPrefix(params.Schema)

	sql := `SELECT name
FROM ` + schemaPrefix + `river_queue
ORDER BY name ASC
LIMIT $1`

	rows, err := e.getDB().Query(ctx, sql, int32(params.Max))
	if err != nil {
		return nil, interpretError(err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return names, nil
}

func (e *Executor) QueuePause(ctx context.Context, params *queue.QueuePauseParams) error {
	schemaPrefix := e.getSchemaPrefix(params.Schema)

	sql := `UPDATE ` + schemaPrefix + `river_queue
SET
    paused_at = CASE WHEN paused_at IS NULL THEN coalesce($1::timestamptz, now()) ELSE paused_at END,
    updated_at = CASE WHEN paused_at IS NULL THEN coalesce($1::timestamptz, now()) ELSE updated_at END
WHERE name = $2::text`

	_, err := e.getDB().Exec(ctx, sql, params.Now, params.Name)
	return interpretError(err)
}

func (e *Executor) QueueResume(ctx context.Context, params *queue.QueueResumeParams) error {
	schemaPrefix := e.getSchemaPrefix(params.Schema)

	sql := `UPDATE ` + schemaPrefix + `river_queue
SET
    paused_at = NULL,
    updated_at = CASE WHEN paused_at IS NOT NULL THEN coalesce($1::timestamptz, now()) ELSE updated_at END
WHERE name = $2::text`

	_, err := e.getDB().Exec(ctx, sql, params.Now, params.Name)
	return interpretError(err)
}

func (e *Executor) QueryRow(ctx context.Context, sql string, args ...any) queue.Row {
	return e.getDB().QueryRow(ctx, sql, args...)
}

type ExecutorTx struct {
	Executor
	tx pgx.Tx
}

func (t *ExecutorTx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

func (t *ExecutorTx) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}
