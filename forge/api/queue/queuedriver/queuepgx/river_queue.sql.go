package queuepgx

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"gamepanel/forge/queue"
)

type dbtx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const schemaPlaceholder = "/* TEMPLATE: schema */"

func prefixSchema(schema string) string {
	if schema == "" {
		return ""
	}
	return safeIdentifier(schema) + "."
}

func replaceSchema(sql, schema string) string {
	return strings.ReplaceAll(sql, schemaPlaceholder, prefixSchema(schema))
}

func safeIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

type jobRowScan struct {
	ID           int64
	Args         []byte
	Attempt      int16
	AttemptedAt  *time.Time
	AttemptedBy  []string
	CreatedAt    time.Time
	Errors       [][]byte
	FinalizedAt  *time.Time
	Kind         string
	MaxAttempts  int16
	Metadata     []byte
	Priority     int16
	Queue        string
	State        string
	ScheduledAt  time.Time
	Tags         []string
	UniqueKey    []byte
	UniqueStates pgtype.Bits
}

func jobRowFromScan(s *jobRowScan) (*queue.JobRow, error) {
	errors := make([]queue.AttemptError, len(s.Errors))
	for i, e := range s.Errors {
		if err := json.Unmarshal(e, &errors[i]); err != nil {
			return nil, err
		}
	}

	return &queue.JobRow{
		ID:          s.ID,
		Attempt:     int(s.Attempt),
		AttemptedAt: s.AttemptedAt,
		AttemptedBy: s.AttemptedBy,
		CreatedAt:   s.CreatedAt,
		EncodedArgs: s.Args,
		Errors:      errors,
		FinalizedAt: s.FinalizedAt,
		Kind:        s.Kind,
		MaxAttempts: int(s.MaxAttempts),
		Metadata:    s.Metadata,
		Priority:    int(s.Priority),
		Queue:       s.Queue,
		ScheduledAt: s.ScheduledAt,
		State:       queue.JobState(s.State),
		Tags:        s.Tags,
		UniqueKey:   s.UniqueKey,
		UniqueStates: func() []queue.JobState {
			if !s.UniqueStates.Valid || len(s.UniqueStates.Bytes) == 0 {
				return nil
			}
			return bitmaskToStates(s.UniqueStates.Bytes[0])
		}(),
	}, nil
}

func bitmaskToStates(b byte) []queue.JobState {
	allStates := queue.JobStates()
	var states []queue.JobState
	for i := 0; i < 8; i++ {
		if b&(1<<i) != 0 && i < len(allStates) {
			states = append(states, allStates[i])
		}
	}
	return states
}

func statesToBitmask(states []queue.JobState) byte {
	var b byte
	allStates := queue.JobStates()
	for _, s := range states {
		for i, as := range allStates {
			if s == as {
				b |= 1 << i
				break
			}
		}
	}
	return b
}

func scanJobRow(row pgx.Row) (*queue.JobRow, error) {
	var s jobRowScan
	err := row.Scan(
		&s.ID,
		&s.Args,
		&s.Attempt,
		&s.AttemptedAt,
		&s.AttemptedBy,
		&s.CreatedAt,
		&s.Errors,
		&s.FinalizedAt,
		&s.Kind,
		&s.MaxAttempts,
		&s.Metadata,
		&s.Priority,
		&s.Queue,
		&s.State,
		&s.ScheduledAt,
		&s.Tags,
		&s.UniqueKey,
		&s.UniqueStates,
	)
	if err != nil {
		return nil, err
	}
	return jobRowFromScan(&s)
}

func scanJobRows(rows pgx.Rows) ([]*queue.JobRow, error) {
	var jobs []*queue.JobRow
	for rows.Next() {
		var s jobRowScan
		err := rows.Scan(
			&s.ID,
			&s.Args,
			&s.Attempt,
			&s.AttemptedAt,
			&s.AttemptedBy,
			&s.CreatedAt,
			&s.Errors,
			&s.FinalizedAt,
			&s.Kind,
			&s.MaxAttempts,
			&s.Metadata,
			&s.Priority,
			&s.Queue,
			&s.State,
			&s.ScheduledAt,
			&s.Tags,
			&s.UniqueKey,
			&s.UniqueStates,
		)
		if err != nil {
			return nil, err
		}
		job, err := jobRowFromScan(&s)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

const jobGetAvailableSQL = `WITH locked_jobs AS (
    SELECT id, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, kind, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states
    FROM /* TEMPLATE: schema */river_job
    WHERE state = 'available'
        AND queue = $1::text
        AND scheduled_at <= coalesce($2::timestamptz, now())
    ORDER BY priority ASC, scheduled_at ASC, id ASC
    LIMIT $3::integer
    FOR UPDATE
    SKIP LOCKED
)
UPDATE /* TEMPLATE: schema */river_job
SET
    state = 'running',
    attempt = river_job.attempt + 1,
    attempted_at = coalesce($2::timestamptz, now()),
    attempted_by = array_append(
        CASE WHEN array_length(river_job.attempted_by, 1) >= 1
        THEN river_job.attempted_by[array_length(river_job.attempted_by, 1) + 2 - 1:]
        ELSE river_job.attempted_by
        END,
        $4::text
    )
FROM locked_jobs
WHERE river_job.id = locked_jobs.id
RETURNING river_job.id, river_job.args, river_job.attempt, river_job.attempted_at, river_job.attempted_by, river_job.created_at, river_job.errors, river_job.finalized_at, river_job.kind, river_job.max_attempts, river_job.metadata, river_job.priority, river_job.queue, river_job.state, river_job.scheduled_at, river_job.tags, river_job.unique_key, river_job.unique_states`

func queryJobGetAvailable(ctx context.Context, db dbtx, schema string, params *queue.JobGetAvailableParams) ([]*queue.JobRow, error) {
	sql := replaceSchema(jobGetAvailableSQL, schema)
	now := params.Now
	if now == nil {
		t := time.Now()
		now = &t
	}
	rows, err := db.Query(ctx, sql, params.Queue, now, int32(params.MaxToLock), params.ClientID)
	if err != nil {
		return nil, interpretError(err)
	}
	defer rows.Close()
	return scanJobRows(rows)
}

const jobInsertFullSQL = `INSERT INTO /* TEMPLATE: schema */river_job(
    args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at,
    kind, max_attempts, metadata, priority, queue, scheduled_at, state, tags,
    unique_key, unique_states
) VALUES (
    $1::jsonb,
    coalesce($2::smallint, 0),
    NULL,
    '{}',
    NOW(),
    '{}',
    NULL,
    $3,
    $4::smallint,
    coalesce($5::jsonb, '{}'),
    $6::smallint,
    $7,
    coalesce($8::timestamptz, NOW()),
    $9::/* TEMPLATE: schema */river_job_state,
    coalesce($10::varchar(255)[], '{}'),
    nullif($11::text, '')::bytea,
    nullif($12::integer, 0)::bit(8)
) RETURNING id, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, kind, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states`

func queryJobInsertFull(ctx context.Context, db dbtx, schema string, params *queue.JobInsertFullParams) (*queue.JobRow, error) {
	sql := replaceSchema(jobInsertFullSQL, schema)

	tags := params.Tags
	if tags == nil {
		tags = []string{}
	}

	var uniqueKeyStr string
	if params.UniqueKey != nil {
		uniqueKeyStr = string(params.UniqueKey)
	}

	row := db.QueryRow(ctx, sql,
		params.EncodedArgs,
		int16(params.Attempt),
		params.Kind,
		int16(params.MaxAttempts),
		params.Metadata,
		int16(params.Priority),
		params.Queue,
		params.ScheduledAt,
		string(params.State),
		tags,
		uniqueKeyStr,
		int32(params.UniqueStates),
	)
	return scanJobRow(row)
}

const jobSetStateIfRunningManySQL = `WITH job_input AS (
    SELECT
        unnest($1::bigint[]) AS id,
        unnest($2::boolean[]) AS attempt_do_update,
        unnest($3::int[]) AS attempt,
        unnest($4::boolean[]) AS errors_do_update,
        unnest($5::jsonb[]) AS errors,
        unnest($6::boolean[]) AS finalized_at_do_update,
        unnest($7::timestamptz[]) AS finalized_at,
        unnest($8::boolean[]) AS metadata_do_merge,
        unnest($9::jsonb[]) AS metadata_updates,
        unnest($10::boolean[]) AS scheduled_at_do_update,
        unnest($11::timestamptz[]) AS scheduled_at,
        unnest($12::text[])::/* TEMPLATE: schema */river_job_state AS state
),
updated AS (
    UPDATE /* TEMPLATE: schema */river_job
    SET
        attempt = CASE
            WHEN river_job.state = 'running'
                 AND NOT (job_input.state IN ('retryable','scheduled') AND river_job.metadata ? 'cancel_attempted_at')
                 AND job_input.attempt_do_update
            THEN job_input.attempt
            ELSE river_job.attempt
        END,
        errors = CASE
            WHEN river_job.state = 'running'
                 AND job_input.errors_do_update
            THEN array_append(river_job.errors, job_input.errors)
            ELSE river_job.errors
        END,
        finalized_at = CASE
            WHEN river_job.state = 'running'
                 AND (job_input.state IN ('retryable','scheduled') AND river_job.metadata ? 'cancel_attempted_at')
            THEN coalesce($13::timestamptz, now())
            WHEN river_job.state = 'running'
                 AND job_input.finalized_at_do_update
            THEN job_input.finalized_at
            ELSE river_job.finalized_at
        END,
        metadata = CASE
            WHEN job_input.metadata_do_merge
            THEN river_job.metadata || job_input.metadata_updates
            ELSE river_job.metadata
        END,
        scheduled_at = CASE
            WHEN river_job.state = 'running'
                 AND NOT (job_input.state IN ('retryable','scheduled') AND river_job.metadata ? 'cancel_attempted_at')
                 AND job_input.scheduled_at_do_update
            THEN job_input.scheduled_at
            ELSE river_job.scheduled_at
        END,
        state = CASE
            WHEN river_job.state = 'running'
                 AND (job_input.state IN ('retryable','scheduled') AND river_job.metadata ? 'cancel_attempted_at')
            THEN 'cancelled'::/* TEMPLATE: schema */river_job_state
            WHEN river_job.state = 'running'
            THEN job_input.state
            ELSE river_job.state
        END
    FROM job_input
    WHERE river_job.id = job_input.id
      AND (river_job.state = 'running' OR job_input.metadata_do_merge)
    RETURNING river_job.id, river_job.args, river_job.attempt, river_job.attempted_at, river_job.attempted_by, river_job.created_at, river_job.errors, river_job.finalized_at, river_job.kind, river_job.max_attempts, river_job.metadata, river_job.priority, river_job.queue, river_job.state, river_job.scheduled_at, river_job.tags, river_job.unique_key, river_job.unique_states
)
SELECT river_job.id, river_job.args, river_job.attempt, river_job.attempted_at, river_job.attempted_by, river_job.created_at, river_job.errors, river_job.finalized_at, river_job.kind, river_job.max_attempts, river_job.metadata, river_job.priority, river_job.queue, river_job.state, river_job.scheduled_at, river_job.tags, river_job.unique_key, river_job.unique_states
FROM /* TEMPLATE: schema */river_job
JOIN job_input ON river_job.id = job_input.id
WHERE NOT EXISTS (
    SELECT 1 FROM updated WHERE updated.id = river_job.id
)
UNION ALL
SELECT id, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, kind, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states
FROM updated
ORDER BY id`

func queryJobSetStateIfRunningMany(ctx context.Context, db dbtx, schema string, params *queue.JobSetStateIfRunningManyParams) ([]*queue.JobRow, error) {
	sql := replaceSchema(jobSetStateIfRunningManySQL, schema)

	n := len(params.Jobs)
	ids := make([]int64, n)
	attemptDoUpdate := make([]bool, n)
	attempt := make([]int32, n)
	errorsDoUpdate := make([]bool, n)
	errors := make([][]byte, n)
	finalizedAtDoUpdate := make([]bool, n)
	finalizedAt := make([]time.Time, n)
	metadataDoMerge := make([]bool, n)
	metadataUpdates := make([][]byte, n)
	scheduledAtDoUpdate := make([]bool, n)
	scheduledAt := make([]time.Time, n)
	states := make([]string, n)

	var now *time.Time
	for i, p := range params.Jobs {
		if p.FinalizedAt != nil && now == nil {
			now = p.FinalizedAt
		}
		ids[i] = p.ID
		attemptDoUpdate[i] = p.Attempt != nil
		if p.Attempt != nil {
			attempt[i] = int32(*p.Attempt)
		}
		errorsDoUpdate[i] = p.ErrData != nil
		if p.ErrData != nil {
			errors[i] = p.ErrData
		}
		finalizedAtDoUpdate[i] = p.FinalizedAt != nil
		if p.FinalizedAt != nil {
			finalizedAt[i] = *p.FinalizedAt
		}
		metadataDoMerge[i] = p.MetadataDoMerge
		if p.MetadataUpdates != nil {
			metadataUpdates[i] = p.MetadataUpdates
		}
		scheduledAtDoUpdate[i] = p.ScheduledAt != nil
		if p.ScheduledAt != nil {
			scheduledAt[i] = *p.ScheduledAt
		}
		states[i] = string(p.State)
	}
	if now == nil {
		t := time.Now()
		now = &t
	}

	rows, err := db.Query(ctx, sql,
		ids,
		attemptDoUpdate,
		attempt,
		errorsDoUpdate,
		errors,
		finalizedAtDoUpdate,
		finalizedAt,
		metadataDoMerge,
		metadataUpdates,
		scheduledAtDoUpdate,
		scheduledAt,
		states,
		now,
	)
	if err != nil {
		return nil, interpretError(err)
	}
	defer rows.Close()
	return scanJobRows(rows)
}

const jobScheduleSQL = `WITH jobs_to_schedule AS (
    SELECT id, unique_key, unique_states, priority, scheduled_at
    FROM /* TEMPLATE: schema */river_job
    WHERE
        state IN ('retryable', 'scheduled')
        AND priority >= 0
        AND scheduled_at <= coalesce($1::timestamptz, now())
    ORDER BY priority, scheduled_at, id
    LIMIT $2::bigint
    FOR UPDATE
),
jobs_with_rownum AS (
    SELECT id, unique_key, unique_states, priority, scheduled_at,
        CASE
            WHEN unique_key IS NOT NULL AND unique_states IS NOT NULL THEN
                ROW_NUMBER() OVER (PARTITION BY unique_key ORDER BY priority, scheduled_at, id)
            ELSE NULL
        END AS row_num
    FROM jobs_to_schedule
),
unique_conflicts AS (
    SELECT river_job.unique_key
    FROM /* TEMPLATE: schema */river_job
    JOIN jobs_with_rownum ON river_job.unique_key = jobs_with_rownum.unique_key
        AND river_job.id != jobs_with_rownum.id
    WHERE
        river_job.unique_key IS NOT NULL
        AND river_job.unique_states IS NOT NULL
        AND /* TEMPLATE: schema */river_job_state_in_bitmask(river_job.unique_states, river_job.state)
),
job_updates AS (
    SELECT
        job.id, job.unique_key, job.unique_states,
        CASE
            WHEN job.row_num IS NULL THEN 'available'::/* TEMPLATE: schema */river_job_state
            WHEN uc.unique_key IS NOT NULL THEN 'discarded'::/* TEMPLATE: schema */river_job_state
            WHEN job.row_num = 1 THEN 'available'::/* TEMPLATE: schema */river_job_state
            ELSE 'discarded'::/* TEMPLATE: schema */river_job_state
        END AS new_state,
        (job.row_num IS NOT NULL AND (uc.unique_key IS NOT NULL OR job.row_num > 1)) AS finalized_at_do_update,
        (job.row_num IS NOT NULL AND (uc.unique_key IS NOT NULL OR job.row_num > 1)) AS metadata_do_update
    FROM jobs_with_rownum job
    LEFT JOIN unique_conflicts uc ON job.unique_key = uc.unique_key
),
updated_jobs AS (
    UPDATE /* TEMPLATE: schema */river_job
    SET
        state        = job_updates.new_state,
        finalized_at = CASE WHEN job_updates.finalized_at_do_update THEN coalesce($1::timestamptz, now())
                            ELSE river_job.finalized_at END,
        metadata     = CASE WHEN job_updates.metadata_do_update THEN river_job.metadata || '{"unique_key_conflict": "scheduler_discarded"}'::jsonb
                            ELSE river_job.metadata END
    FROM job_updates
    WHERE river_job.id = job_updates.id
    RETURNING
        river_job.id,
        job_updates.new_state = 'discarded'::/* TEMPLATE: schema */river_job_state AS conflict_discarded
)
SELECT
    river_job.id, river_job.args, river_job.attempt, river_job.attempted_at, river_job.attempted_by, river_job.created_at, river_job.errors, river_job.finalized_at, river_job.kind, river_job.max_attempts, river_job.metadata, river_job.priority, river_job.queue, river_job.state, river_job.scheduled_at, river_job.tags, river_job.unique_key, river_job.unique_states,
    updated_jobs.conflict_discarded
FROM /* TEMPLATE: schema */river_job
JOIN updated_jobs ON river_job.id = updated_jobs.id`

type jobScheduleRowScan struct {
	jobRowScan
	ConflictDiscarded bool
}

func queryJobSchedule(ctx context.Context, db dbtx, schema string, params *queue.JobScheduleParams) ([]*queue.JobScheduleResult, error) {
	sql := replaceSchema(jobScheduleSQL, schema)
	now := params.Now
	if now == nil {
		t := time.Now()
		now = &t
	}
	rows, err := db.Query(ctx, sql, now, int64(params.Max))
	if err != nil {
		return nil, interpretError(err)
	}
	defer rows.Close()

	var results []*queue.JobScheduleResult
	for rows.Next() {
		var s jobScheduleRowScan
		err := rows.Scan(
			&s.ID,
			&s.Args,
			&s.Attempt,
			&s.AttemptedAt,
			&s.AttemptedBy,
			&s.CreatedAt,
			&s.Errors,
			&s.FinalizedAt,
			&s.Kind,
			&s.MaxAttempts,
			&s.Metadata,
			&s.Priority,
			&s.Queue,
			&s.State,
			&s.ScheduledAt,
			&s.Tags,
			&s.UniqueKey,
			&s.UniqueStates,
			&s.ConflictDiscarded,
		)
		if err != nil {
			return nil, interpretError(err)
		}
		job, err := jobRowFromScan(&s.jobRowScan)
		if err != nil {
			return nil, err
		}
		results = append(results, &queue.JobScheduleResult{
			Job:               *job,
			ConflictDiscarded: s.ConflictDiscarded,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

const jobCancelSQL = `UPDATE /* TEMPLATE: schema */river_job
SET
    state = 'cancelled',
    finalized_at = coalesce($2::timestamptz, now())
WHERE id = $1
    AND state NOT IN ('cancelled', 'completed', 'discarded')
RETURNING id, args, attempt, attempted_at, attempted_by, created_at, errors, finalized_at, kind, max_attempts, metadata, priority, queue, state, scheduled_at, tags, unique_key, unique_states`

func queryJobCancel(ctx context.Context, db dbtx, schema string, params *queue.JobCancelParams) (*queue.JobRow, error) {
	sql := replaceSchema(jobCancelSQL, schema)
	now := time.Now()
	row := db.QueryRow(ctx, sql, params.ID, now)
	job, err := scanJobRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, queue.ErrNotFound
		}
		return nil, interpretError(err)
	}
	return job, nil
}

const leaderAttemptElectSQL = `INSERT INTO /* TEMPLATE: schema */river_leader (
    leader_id, elected_at, expires_at, name
) VALUES (
    $1, coalesce($2::timestamptz, now()),
    coalesce($2::timestamptz, now()) + make_interval(secs => $3),
    $4
)
ON CONFLICT (name)
    DO NOTHING
RETURNING elected_at, expires_at, leader_id, name`

type leaderRowScan struct {
	ElectedAt time.Time
	ExpiresAt time.Time
	LeaderID  string
	Name      string
}

func queryLeaderAttemptElect(ctx context.Context, db dbtx, schema string, params *queue.LeaderElectParams) (*queue.Leader, error) {
	sql := replaceSchema(leaderAttemptElectSQL, schema)
	now := params.Now
	if now == nil {
		t := time.Now()
		now = &t
	}
	row := db.QueryRow(ctx, sql, params.LeaderID, now, params.TTL.Seconds(), params.LeaderID)
	var s leaderRowScan
	err := row.Scan(&s.ElectedAt, &s.ExpiresAt, &s.LeaderID, &s.Name)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, interpretError(err)
	}
	return &queue.Leader{
		ElectedAt: s.ElectedAt,
		ExpiresAt: s.ExpiresAt,
		LeaderID:  s.LeaderID,
	}, nil
}

const notifyManySQL = `SELECT pg_notify(
    concat(coalesce($1::text, current_schema()), '.', $2::text),
    unnest($3::text[])
)`

func queryNotifyMany(ctx context.Context, db dbtx, schema string, params *queue.NotifyManyParams) error {
	sql := "SELECT pg_notify(concat(coalesce($1::text, current_schema()), '.', $2::text), unnest($3::text[]))"
	_, err := db.Exec(ctx, sql, schema, params.Topic, params.Payload)
	return interpretError(err)
}

const queueCreateOrSetUpdatedAtSQL = `INSERT INTO /* TEMPLATE: schema */river_queue (
    created_at, name, paused_at, updated_at
) VALUES (
    coalesce($1::timestamptz, now()), $2::text, $3::timestamptz,
    coalesce($4::timestamptz, $1::timestamptz, now())
) ON CONFLICT (name) DO UPDATE
SET updated_at = EXCLUDED.updated_at
RETURNING name`

func queryQueueCreateOrSetUpdatedAt(ctx context.Context, db dbtx, schema string, params *queue.QueueCreateOrSetUpdatedAtParams) error {
	sql := replaceSchema(queueCreateOrSetUpdatedAtSQL, schema)
	_, err := db.Exec(ctx, sql, params.Now, params.Name, params.PausedAt, params.UpdatedAt)
	return interpretError(err)
}

func interpretError(err error) error {
	if err == nil {
		return nil
	}
	if err == pgx.ErrNoRows {
		return queue.ErrNotFound
	}
	return err
}
