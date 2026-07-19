package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const ScheduleLeaseDuration = 5 * time.Minute

type ClaimedSchedule struct {
	Schedule Schedule
	RunID    string
	WorkerID string
}

func (s *Store) ClaimDueSchedule(ctx context.Context, now time.Time, workerID string) (*ClaimedSchedule, error) {
	return s.claimSchedule(ctx, now, workerID, "scheduler", "")
}

func (s *Store) ClaimManualSchedule(ctx context.Context, now time.Time, workerID, serverID, scheduleID string) (*ClaimedSchedule, error) {
	return s.claimSchedule(ctx, now, workerID, "manual", scheduleID+":"+serverID)
}

func (s *Store) claimSchedule(ctx context.Context, now time.Time, workerID, trigger, manualKey string) (*ClaimedSchedule, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// Expired ownership is explicitly recorded before a replacement run is claimed.
	_, err = tx.Exec(ctx, `UPDATE schedule_runs SET status='failed', error='worker lease expired before completion', finished_at=now() WHERE status='running' AND lease_expires_at < now()`)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE server_schedules SET lease_owner=NULL, lease_expires_at=NULL WHERE lease_expires_at < now()`)
	if err != nil {
		return nil, err
	}
	var schedule Schedule
	query := `SELECT ss.id::text, ss.server_id::text, ss.name, ss.cron_minute, ss.cron_hour, ss.cron_day_of_month, ss.cron_month, ss.cron_day_of_week, ss.only_when_online, ss.enabled, ss.last_run_at, ss.next_run_at, ss.created_at, ss.updated_at FROM server_schedules ss JOIN servers s ON s.id=ss.server_id WHERE ss.enabled=TRUE AND ss.next_run_at IS NOT NULL AND ss.next_run_at <= $1 AND ss.lease_owner IS NULL AND (ss.only_when_online=FALSE OR s.status='running') ORDER BY ss.next_run_at, ss.created_at FOR UPDATE OF ss SKIP LOCKED LIMIT 1`
	args := []any{now.UTC()}
	if manualKey != "" {
		parts := splitManualKey(manualKey)
		query = `SELECT ss.id::text, ss.server_id::text, ss.name, ss.cron_minute, ss.cron_hour, ss.cron_day_of_month, ss.cron_month, ss.cron_day_of_week, ss.only_when_online, ss.enabled, ss.last_run_at, ss.next_run_at, ss.created_at, ss.updated_at FROM server_schedules ss JOIN servers s ON s.id=ss.server_id WHERE ss.id=$1 AND ss.server_id=$2 AND ss.lease_owner IS NULL AND (ss.only_when_online=FALSE OR s.status='running') FOR UPDATE OF ss SKIP LOCKED`
		args = []any{parts[0], parts[1]}
	}
	err = tx.QueryRow(ctx, query, args...).Scan(&schedule.ID, &schedule.ServerID, &schedule.Name, &schedule.CronMinute, &schedule.CronHour, &schedule.CronDayOfMonth, &schedule.CronMonth, &schedule.CronDayOfWeek, &schedule.OnlyWhenOnline, &schedule.Enabled, &schedule.LastRunAt, &schedule.NextRunAt, &schedule.CreatedAt, &schedule.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	leaseUntil := now.UTC().Add(ScheduleLeaseDuration)
	runID := uuid.NewString()
	if _, err := tx.Exec(ctx, `UPDATE server_schedules SET lease_owner=$2, lease_expires_at=$3 WHERE id=$1`, schedule.ID, workerID, leaseUntil); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schedule_runs (id,schedule_id,server_id,status,trigger,worker_id,lease_expires_at) VALUES ($1,$2,$3,'running',$4,$5,$6)`, runID, schedule.ID, schedule.ServerID, trigger, workerID, leaseUntil); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text,schedule_id::text,sequence,action,payload,time_offset_seconds,continue_on_failure,created_at FROM schedule_tasks WHERE schedule_id=$1 ORDER BY sequence`, schedule.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var task ScheduleTask
		var raw []byte
		if err := rows.Scan(&task.ID, &task.ScheduleID, &task.Sequence, &task.Action, &raw, &task.TimeOffsetSeconds, &task.ContinueOnFailure, &task.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		task.Payload = map[string]any{}
		_ = json.Unmarshal(raw, &task.Payload)
		schedule.Tasks = append(schedule.Tasks, task)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ClaimedSchedule{Schedule: schedule, RunID: runID, WorkerID: workerID}, nil
}

func splitManualKey(value string) [2]string {
	for i := range value {
		if value[i] == ':' {
			return [2]string{value[:i], value[i+1:]}
		}
	}
	return [2]string{value, ""}
}

func (s *Store) ExtendScheduleLease(ctx context.Context, scheduleID, runID, workerID string, until time.Time) error {
	cmd, err := s.db.Exec(ctx, `WITH updated_schedule AS (UPDATE server_schedules SET lease_expires_at=$4 WHERE id=$1 AND lease_owner=$3 RETURNING id) UPDATE schedule_runs SET lease_expires_at=$4 WHERE id=$2 AND worker_id=$3 AND status='running' AND EXISTS (SELECT 1 FROM updated_schedule)`, scheduleID, runID, workerID, until.UTC())
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("schedule lease is no longer owned by this worker")
	}
	return nil
}

func (s *Store) FinishScheduleClaim(ctx context.Context, claimed ClaimedSchedule, status ScheduleRunStatus, errMessage *string, lastRunAt, nextRunAt time.Time) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	cmd, err := tx.Exec(ctx, `UPDATE schedule_runs SET status=$4,error=$5,finished_at=now(),lease_expires_at=NULL WHERE id=$1 AND schedule_id=$2 AND worker_id=$3 AND status='running'`, claimed.RunID, claimed.Schedule.ID, claimed.WorkerID, status, errMessage)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("schedule run lease was lost")
	}
	cmd, err = tx.Exec(ctx, `UPDATE server_schedules SET last_run_at=$3,next_run_at=$4,lease_owner=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$1 AND lease_owner=$2`, claimed.Schedule.ID, claimed.WorkerID, lastRunAt.UTC(), nextRunAt.UTC())
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("schedule lease was lost")
	}
	return tx.Commit(ctx)
}

func (s *Store) FailScheduleClaim(ctx context.Context, claimed ClaimedSchedule, message string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE schedule_runs SET status='failed',error=$4,finished_at=now(),lease_expires_at=NULL WHERE id=$1 AND schedule_id=$2 AND worker_id=$3 AND status='running'`, claimed.RunID, claimed.Schedule.ID, claimed.WorkerID, message)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE server_schedules SET lease_owner=NULL,lease_expires_at=NULL WHERE id=$1 AND lease_owner=$2`, claimed.Schedule.ID, claimed.WorkerID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
