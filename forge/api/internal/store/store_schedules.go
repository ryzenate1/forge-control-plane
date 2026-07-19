package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

func resolveStartupCommand(raw string, env map[string]string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	resolved := raw
	for key, value := range env {
		resolved = strings.ReplaceAll(resolved, "{{"+key+"}}", value)
		resolved = strings.ReplaceAll(resolved, "{{"+strings.ToLower(key)+"}}", value)
	}
	return resolved
}

func (s *Store) ListSchedules(ctx context.Context, serverID string) ([]Schedule, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, name, cron_minute, cron_hour, cron_day_of_month, cron_month, cron_day_of_week,
		       timezone, only_when_online, enabled, last_run_at, next_run_at, created_at, updated_at
		FROM server_schedules
		WHERE server_id = $1
		ORDER BY created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schedules := []Schedule{}
	byID := map[string]int{}
	for rows.Next() {
		var schedule Schedule
		if err := rows.Scan(
			&schedule.ID,
			&schedule.ServerID,
			&schedule.Name,
			&schedule.CronMinute,
			&schedule.CronHour,
			&schedule.CronDayOfMonth,
			&schedule.CronMonth,
			&schedule.CronDayOfWeek,
			&schedule.Timezone,
			&schedule.OnlyWhenOnline,
			&schedule.Enabled,
			&schedule.LastRunAt,
			&schedule.NextRunAt,
			&schedule.CreatedAt,
			&schedule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		schedule.Tasks = []ScheduleTask{}
		byID[schedule.ID] = len(schedules)
		schedules = append(schedules, schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(schedules) == 0 {
		return schedules, nil
	}

	taskRows, err := s.db.Query(ctx, `
		SELECT t.id::text, t.schedule_id::text, t.sequence, t.action, t.payload, t.time_offset_seconds, t.continue_on_failure, t.created_at
		FROM schedule_tasks t
		JOIN server_schedules ss ON ss.id = t.schedule_id
		WHERE ss.server_id = $1
		ORDER BY t.sequence ASC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer taskRows.Close()

	for taskRows.Next() {
		var task ScheduleTask
		var payloadRaw []byte
		if err := taskRows.Scan(&task.ID, &task.ScheduleID, &task.Sequence, &task.Action, &payloadRaw, &task.TimeOffsetSeconds, &task.ContinueOnFailure, &task.CreatedAt); err != nil {
			return nil, err
		}
		task.Payload = map[string]any{}
		if len(payloadRaw) > 0 {
			_ = json.Unmarshal(payloadRaw, &task.Payload)
		}
		if index, ok := byID[task.ScheduleID]; ok {
			schedules[index].Tasks = append(schedules[index].Tasks, task)
		}
	}
	if err := taskRows.Err(); err != nil {
		return nil, err
	}

	return schedules, nil
}

func (s *Store) GetSchedule(ctx context.Context, serverID, scheduleID string) (Schedule, error) {
	var schedule Schedule
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, name, cron_minute, cron_hour, cron_day_of_month, cron_month, cron_day_of_week,
		       timezone, only_when_online, enabled, last_run_at, next_run_at, created_at, updated_at
		FROM server_schedules
		WHERE id = $1 AND server_id = $2
	`, scheduleID, serverID).Scan(
		&schedule.ID,
		&schedule.ServerID,
		&schedule.Name,
		&schedule.CronMinute,
		&schedule.CronHour,
		&schedule.CronDayOfMonth,
		&schedule.CronMonth,
		&schedule.CronDayOfWeek,
		&schedule.Timezone,
		&schedule.OnlyWhenOnline,
		&schedule.Enabled,
		&schedule.LastRunAt,
		&schedule.NextRunAt,
		&schedule.CreatedAt,
		&schedule.UpdatedAt,
	)
	if err != nil {
		return Schedule{}, err
	}

	taskRows, err := s.db.Query(ctx, `
		SELECT id::text, schedule_id::text, sequence, action, payload, time_offset_seconds, continue_on_failure, created_at
		FROM schedule_tasks
		WHERE schedule_id = $1
		ORDER BY sequence ASC
	`, scheduleID)
	if err != nil {
		return Schedule{}, err
	}
	defer taskRows.Close()

	tasks := []ScheduleTask{}
	for taskRows.Next() {
		var task ScheduleTask
		var payloadRaw []byte
		if err := taskRows.Scan(&task.ID, &task.ScheduleID, &task.Sequence, &task.Action, &payloadRaw, &task.TimeOffsetSeconds, &task.ContinueOnFailure, &task.CreatedAt); err != nil {
			return Schedule{}, err
		}
		task.Payload = map[string]any{}
		if len(payloadRaw) > 0 {
			_ = json.Unmarshal(payloadRaw, &task.Payload)
		}
		tasks = append(tasks, task)
	}
	if err := taskRows.Err(); err != nil {
		return Schedule{}, err
	}
	schedule.Tasks = tasks

	return schedule, nil
}

func (s *Store) CreateSchedule(ctx context.Context, serverID string, req CreateScheduleRequest, actorID *string) (Schedule, error) {
	if strings.TrimSpace(req.Name) == "" {
		return Schedule{}, errors.New("name is required")
	}
	if strings.TrimSpace(req.CronMinute) == "" {
		req.CronMinute = "*"
	}
	if strings.TrimSpace(req.CronHour) == "" {
		req.CronHour = "*"
	}
	if strings.TrimSpace(req.CronDayOfMonth) == "" {
		req.CronDayOfMonth = "*"
	}
	if strings.TrimSpace(req.CronMonth) == "" {
		req.CronMonth = "*"
	}
	if strings.TrimSpace(req.CronDayOfWeek) == "" {
		req.CronDayOfWeek = "*"
	}
	if strings.TrimSpace(req.Timezone) == "" {
		req.Timezone = "UTC"
	}

	scheduleID := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO server_schedules (
			id, server_id, name, cron_minute, cron_hour, cron_day_of_month, cron_month, cron_day_of_week,
			timezone, only_when_online, enabled
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, scheduleID, serverID, strings.TrimSpace(req.Name), req.CronMinute, req.CronHour, req.CronDayOfMonth, req.CronMonth, req.CronDayOfWeek, req.Timezone, req.OnlyWhenOnline, req.Enabled)
	if err != nil {
		return Schedule{}, err
	}
	_ = s.AppendAudit(ctx, actorID, "schedule created", "server", &serverID, fmt.Sprintf(`{"scheduleId":"%s"}`, scheduleID))
	schedule, err := s.GetSchedule(ctx, serverID, scheduleID)
	if err == nil {
		_ = s.NotifySchedulesChanged(ctx)
	}
	return schedule, err
}

func (s *Store) PatchSchedule(ctx context.Context, serverID, scheduleID string, req PatchScheduleRequest, actorID *string) (Schedule, error) {
	commandTag, err := s.db.Exec(ctx, `
		UPDATE server_schedules
		SET name = COALESCE($1, name),
		    cron_minute = COALESCE($2, cron_minute),
		    cron_hour = COALESCE($3, cron_hour),
		    cron_day_of_month = COALESCE($4, cron_day_of_month),
		    cron_month = COALESCE($5, cron_month),
		    cron_day_of_week = COALESCE($6, cron_day_of_week),
		    timezone = COALESCE($7, timezone),
		    only_when_online = COALESCE($8, only_when_online),
		    enabled = COALESCE($9, enabled),
		    updated_at = now()
		WHERE id = $10 AND server_id = $11
	`, req.Name, req.CronMinute, req.CronHour, req.CronDayOfMonth, req.CronMonth, req.CronDayOfWeek, req.Timezone, req.OnlyWhenOnline, req.Enabled, scheduleID, serverID)
	if err != nil {
		return Schedule{}, err
	}
	if commandTag.RowsAffected() == 0 {
		return Schedule{}, errors.New("schedule not found")
	}
	_ = s.AppendAudit(ctx, actorID, "schedule updated", "server", &serverID, fmt.Sprintf(`{"scheduleId":"%s"}`, scheduleID))
	schedule, err := s.GetSchedule(ctx, serverID, scheduleID)
	if err == nil {
		_ = s.NotifySchedulesChanged(ctx)
	}
	return schedule, err
}

func (s *Store) DeleteSchedule(ctx context.Context, serverID, scheduleID string, actorID *string) error {
	commandTag, err := s.db.Exec(ctx, `DELETE FROM server_schedules WHERE id = $1 AND server_id = $2`, scheduleID, serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("schedule not found")
	}
	err = s.AppendAudit(ctx, actorID, "schedule deleted", "server", &serverID, fmt.Sprintf(`{"scheduleId":"%s"}`, scheduleID))
	if err == nil {
		_ = s.NotifySchedulesChanged(ctx)
	}
	return err
}

func (s *Store) CreateScheduleTask(ctx context.Context, serverID, scheduleID string, req CreateScheduleTaskRequest, actorID *string) (ScheduleTask, error) {
	if strings.TrimSpace(req.Action) == "" {
		return ScheduleTask{}, errors.New("action is required")
	}
	if req.TimeOffsetSeconds < 0 {
		return ScheduleTask{}, errors.New("timeOffsetSeconds cannot be negative")
	}
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM server_schedules WHERE id = $1 AND server_id = $2)`, scheduleID, serverID).Scan(&exists); err != nil {
		return ScheduleTask{}, err
	}
	if !exists {
		return ScheduleTask{}, errors.New("schedule not found")
	}
	if req.Sequence <= 0 {
		if err := s.db.QueryRow(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM schedule_tasks WHERE schedule_id = $1`, scheduleID).Scan(&req.Sequence); err != nil {
			return ScheduleTask{}, err
		}
	}
	payload := req.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return ScheduleTask{}, err
	}

	taskID := uuid.NewString()
	_, err = s.db.Exec(ctx, `
		INSERT INTO schedule_tasks (id, schedule_id, sequence, action, payload, time_offset_seconds, continue_on_failure)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
	`, taskID, scheduleID, req.Sequence, strings.TrimSpace(req.Action), string(payloadRaw), req.TimeOffsetSeconds, req.ContinueOnFailure)
	if err != nil {
		return ScheduleTask{}, err
	}
	_ = s.AppendAudit(ctx, actorID, "schedule task created", "server", &serverID, fmt.Sprintf(`{"scheduleId":"%s","taskId":"%s"}`, scheduleID, taskID))

	var task ScheduleTask
	var payloadBytes []byte
	if err := s.db.QueryRow(ctx, `
		SELECT id::text, schedule_id::text, sequence, action, payload, time_offset_seconds, continue_on_failure, created_at
		FROM schedule_tasks
		WHERE id = $1
	`, taskID).Scan(&task.ID, &task.ScheduleID, &task.Sequence, &task.Action, &payloadBytes, &task.TimeOffsetSeconds, &task.ContinueOnFailure, &task.CreatedAt); err != nil {
		return ScheduleTask{}, err
	}
	task.Payload = map[string]any{}
	if len(payloadBytes) > 0 {
		_ = json.Unmarshal(payloadBytes, &task.Payload)
	}
	_ = s.NotifySchedulesChanged(ctx)
	return task, nil
}

func (s *Store) PatchScheduleTask(ctx context.Context, serverID, scheduleID, taskID string, req PatchScheduleTaskRequest, actorID *string) (ScheduleTask, error) {
	if req.Action != nil && !isValidScheduleTaskAction(*req.Action) {
		return ScheduleTask{}, fmt.Errorf("unsupported task action: %s", strings.TrimSpace(*req.Action))
	}
	var payloadArg any
	if req.Payload != nil {
		payloadRaw, err := json.Marshal(*req.Payload)
		if err != nil {
			return ScheduleTask{}, err
		}
		payloadArg = string(payloadRaw)
	}
	commandTag, err := s.db.Exec(ctx, `
		UPDATE schedule_tasks t
		SET sequence = COALESCE($1, t.sequence),
		    action = COALESCE($2, t.action),
		    payload = COALESCE($3::jsonb, t.payload),
		    time_offset_seconds = COALESCE($4, t.time_offset_seconds),
		    continue_on_failure = COALESCE($5, t.continue_on_failure)
		FROM server_schedules ss
		WHERE t.id = $6 AND t.schedule_id = ss.id AND ss.id = $7 AND ss.server_id = $8
	`, req.Sequence, req.Action, payloadArg, req.TimeOffsetSeconds, req.ContinueOnFailure, taskID, scheduleID, serverID)
	if err != nil {
		return ScheduleTask{}, err
	}
	if commandTag.RowsAffected() == 0 {
		return ScheduleTask{}, errors.New("schedule task not found")
	}
	_ = s.AppendAudit(ctx, actorID, "schedule task updated", "server", &serverID, fmt.Sprintf(`{"scheduleId":"%s","taskId":"%s"}`, scheduleID, taskID))

	var task ScheduleTask
	var payloadBytes []byte
	if err := s.db.QueryRow(ctx, `
		SELECT t.id::text, t.schedule_id::text, t.sequence, t.action, t.payload, t.time_offset_seconds, t.continue_on_failure, t.created_at
		FROM schedule_tasks t
		JOIN server_schedules ss ON ss.id = t.schedule_id
		WHERE t.id = $1 AND ss.id = $2 AND ss.server_id = $3
	`, taskID, scheduleID, serverID).Scan(&task.ID, &task.ScheduleID, &task.Sequence, &task.Action, &payloadBytes, &task.TimeOffsetSeconds, &task.ContinueOnFailure, &task.CreatedAt); err != nil {
		return ScheduleTask{}, err
	}
	task.Payload = map[string]any{}
	if len(payloadBytes) > 0 {
		_ = json.Unmarshal(payloadBytes, &task.Payload)
	}
	_ = s.NotifySchedulesChanged(ctx)
	return task, nil
}

func (s *Store) DeleteScheduleTask(ctx context.Context, serverID, scheduleID, taskID string, actorID *string) error {
	commandTag, err := s.db.Exec(ctx, `
		DELETE FROM schedule_tasks t
		USING server_schedules ss
		WHERE t.id = $1 AND t.schedule_id = ss.id AND ss.id = $2 AND ss.server_id = $3
	`, taskID, scheduleID, serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("schedule task not found")
	}
	err = s.AppendAudit(ctx, actorID, "schedule task deleted", "server", &serverID, fmt.Sprintf(`{"scheduleId":"%s","taskId":"%s"}`, scheduleID, taskID))
	if err == nil {
		_ = s.NotifySchedulesChanged(ctx)
	}
	return err
}

func (s *Store) NotifySchedulesChanged(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `NOTIFY schedule_events`)
	return err
}

func (s *Store) ListenScheduleEvents(ctx context.Context) (<-chan struct{}, <-chan error) {
	events := make(chan struct{}, 1)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		const maxBackoff = 30 * time.Second
		backoff := 100 * time.Millisecond

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			conn, err := s.db.Acquire(ctx)
			if err != nil {
				select {
				case errs <- err:
				default:
				}
				time.Sleep(backoff)
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}

			if _, err := conn.Exec(ctx, `LISTEN schedule_events`); err != nil {
				conn.Release()
				select {
				case errs <- err:
				default:
				}
				time.Sleep(backoff)
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}

			backoff = 100 * time.Millisecond

			for {
				if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
					conn.Release()
					if ctx.Err() == nil {
						select {
						case errs <- err:
						default:
						}
					}
					break
				}
				select {
				case events <- struct{}{}:
				default:
				}
			}
		}
	}()

	return events, errs
}

func (s *Store) NextScheduleRunAt(ctx context.Context, now time.Time) (*time.Time, error) {
	var next *time.Time
	if err := s.db.QueryRow(ctx, `
		SELECT MIN(ss.next_run_at)
		FROM server_schedules ss
		JOIN servers s ON s.id = ss.server_id
		WHERE ss.enabled = TRUE
		  AND ss.next_run_at IS NOT NULL
		  AND (ss.only_when_online = FALSE OR s.status = 'running')
		  AND ss.next_run_at >= $1
	`, now.UTC()).Scan(&next); err != nil {
		return nil, err
	}
	return next, nil
}

func (s *Store) ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]Schedule, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT ss.id::text, ss.server_id::text, ss.name, ss.cron_minute, ss.cron_hour, ss.cron_day_of_month, ss.cron_month, ss.cron_day_of_week,
		       ss.only_when_online, ss.enabled, ss.last_run_at, ss.next_run_at, ss.created_at, ss.updated_at
		FROM server_schedules ss
		JOIN servers s ON s.id = ss.server_id
		WHERE ss.enabled = TRUE
		  AND (ss.next_run_at IS NULL OR ss.next_run_at <= $1)
		  AND (ss.only_when_online = FALSE OR s.status = 'running')
		ORDER BY ss.next_run_at NULLS FIRST, ss.created_at ASC
		LIMIT $2
	`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schedules := []Schedule{}
	byID := map[string]int{}
	for rows.Next() {
		var schedule Schedule
		if err := rows.Scan(
			&schedule.ID,
			&schedule.ServerID,
			&schedule.Name,
			&schedule.CronMinute,
			&schedule.CronHour,
			&schedule.CronDayOfMonth,
			&schedule.CronMonth,
			&schedule.CronDayOfWeek,
			&schedule.OnlyWhenOnline,
			&schedule.Enabled,
			&schedule.LastRunAt,
			&schedule.NextRunAt,
			&schedule.CreatedAt,
			&schedule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		schedule.Tasks = []ScheduleTask{}
		byID[schedule.ID] = len(schedules)
		schedules = append(schedules, schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(schedules) == 0 {
		return schedules, nil
	}

	ids := make([]string, 0, len(schedules))
	for _, schedule := range schedules {
		ids = append(ids, schedule.ID)
	}

	taskRows, err := s.db.Query(ctx, `
		SELECT t.id::text, t.schedule_id::text, t.sequence, t.action, t.payload, t.time_offset_seconds, t.continue_on_failure, t.created_at
		FROM schedule_tasks t
		WHERE t.schedule_id = ANY($1::uuid[])
		ORDER BY t.schedule_id, t.sequence ASC
	`, ids)
	if err != nil {
		return nil, err
	}
	defer taskRows.Close()

	for taskRows.Next() {
		var task ScheduleTask
		var payloadRaw []byte
		if err := taskRows.Scan(&task.ID, &task.ScheduleID, &task.Sequence, &task.Action, &payloadRaw, &task.TimeOffsetSeconds, &task.ContinueOnFailure, &task.CreatedAt); err != nil {
			return nil, err
		}
		task.Payload = map[string]any{}
		if len(payloadRaw) > 0 {
			_ = json.Unmarshal(payloadRaw, &task.Payload)
		}
		if index, ok := byID[task.ScheduleID]; ok {
			schedules[index].Tasks = append(schedules[index].Tasks, task)
		}
	}
	if err := taskRows.Err(); err != nil {
		return nil, err
	}

	return schedules, nil
}

func (s *Store) UpdateScheduleRunMeta(ctx context.Context, scheduleID string, lastRunAt, nextRunAt *time.Time) error {
	_, err := s.db.Exec(ctx, `
		UPDATE server_schedules
		SET last_run_at = COALESCE($1, last_run_at),
		    next_run_at = COALESCE($2, next_run_at),
		    updated_at = now()
		WHERE id = $3
	`, lastRunAt, nextRunAt, scheduleID)
	return err
}

func (s *Store) CreateScheduleRun(ctx context.Context, scheduleID, serverID, trigger string) (string, error) {
	runID := uuid.NewString()
	if strings.TrimSpace(trigger) == "" {
		trigger = "scheduler"
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO schedule_runs (id, schedule_id, server_id, status, trigger)
		VALUES ($1, $2, $3, $4, $5)
	`, runID, scheduleID, serverID, string(ScheduleRunRunning), trigger)
	if err != nil {
		return "", err
	}
	return runID, nil
}

func (s *Store) FinishScheduleRun(ctx context.Context, runID string, status ScheduleRunStatus, errMessage *string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE schedule_runs
		SET status = $1,
		    error = $2,
		    finished_at = now()
		WHERE id = $3
	`, string(status), errMessage, runID)
	return err
}

func (s *Store) CreateScheduleTaskRun(ctx context.Context, runID, taskID string, status ScheduleTaskRunStatus, errMessage *string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO schedule_task_runs (id, schedule_run_id, schedule_task_id, status, error)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.NewString(), runID, taskID, string(status), errMessage)
	return err
}

func (s *Store) ListScheduleRuns(ctx context.Context, serverID, scheduleID string, limit int) ([]ScheduleRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	runRows, err := s.db.Query(ctx, `
		SELECT id::text, schedule_id::text, server_id::text, status, trigger, error, started_at, finished_at
		FROM schedule_runs
		WHERE server_id = $1 AND schedule_id = $2
		ORDER BY started_at DESC
		LIMIT $3
	`, serverID, scheduleID, limit)
	if err != nil {
		return nil, err
	}
	defer runRows.Close()

	runs := []ScheduleRun{}
	byID := map[string]int{}
	for runRows.Next() {
		var run ScheduleRun
		if err := runRows.Scan(&run.ID, &run.ScheduleID, &run.ServerID, &run.Status, &run.Trigger, &run.Error, &run.StartedAt, &run.FinishedAt); err != nil {
			return nil, err
		}
		run.Tasks = []ScheduleTaskRun{}
		byID[run.ID] = len(runs)
		runs = append(runs, run)
	}
	if err := runRows.Err(); err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return runs, nil
	}

	ids := make([]string, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.ID)
	}

	taskRows, err := s.db.Query(ctx, `
		SELECT id::text, schedule_run_id::text, schedule_task_id::text, status, error, executed_at
		FROM schedule_task_runs
		WHERE schedule_run_id = ANY($1::uuid[])
		ORDER BY executed_at ASC
	`, ids)
	if err != nil {
		return nil, err
	}
	defer taskRows.Close()

	for taskRows.Next() {
		var taskRun ScheduleTaskRun
		if err := taskRows.Scan(&taskRun.ID, &taskRun.ScheduleRunID, &taskRun.ScheduleTaskID, &taskRun.Status, &taskRun.Error, &taskRun.ExecutedAt); err != nil {
			return nil, err
		}
		if index, ok := byID[taskRun.ScheduleRunID]; ok {
			runs[index].Tasks = append(runs[index].Tasks, taskRun)
		}
	}
	if err := taskRows.Err(); err != nil {
		return nil, err
	}

	return runs, nil
}

func (s *Store) ListScheduleTasks(ctx context.Context, serverID, scheduleID string) ([]ScheduleTask, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id::text, t.schedule_id::text, t.sequence, t.action, t.payload, t.time_offset_seconds, t.continue_on_failure, t.created_at
		FROM schedule_tasks t
		JOIN server_schedules ss ON ss.id = t.schedule_id
		WHERE ss.server_id = $1 AND t.schedule_id = $2
		ORDER BY t.sequence, t.created_at
	`, serverID, scheduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []ScheduleTask{}
	for rows.Next() {
		var task ScheduleTask
		var payload []byte
		if err := rows.Scan(
			&task.ID,
			&task.ScheduleID,
			&task.Sequence,
			&task.Action,
			&payload,
			&task.TimeOffsetSeconds,
			&task.ContinueOnFailure,
			&task.CreatedAt,
		); err != nil {
			return nil, err
		}
		if len(payload) > 0 {
			_ = json.Unmarshal(payload, &task.Payload)
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}
