package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"time"
)

type MaintenanceService interface {
	Start(ctx context.Context)
	Stop()
}

type JobCleaner struct {
	exec      Executor
	schema    string
	logger    *slog.Logger
	stopCh    chan struct{}
	retention time.Duration
}

func NewJobCleaner(exec Executor, schema string, retention time.Duration) *JobCleaner {
	if retention <= 0 {
		retention = 24 * time.Hour
	}
	return &JobCleaner{
		exec:      exec,
		schema:    schema,
		logger:    slog.Default(),
		stopCh:    make(chan struct{}),
		retention: retention,
	}
}

func (c *JobCleaner) Start(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				c.logger.ErrorContext(ctx, "job cleaner panic recovered", "panic", r, "stack", string(buf[:n]))
			}
		}()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-c.stopCh:
				return
			case <-ticker.C:
				c.clean(ctx)
			}
		}
	}()
}

func (c *JobCleaner) Stop() {
	close(c.stopCh)
}

func (c *JobCleaner) clean(ctx context.Context) {
	horizon := time.Now().UTC().Add(-c.retention)

	for _, state := range []JobState{JobStateCompleted, JobStateCancelled, JobStateDiscarded} {
		n, err := c.exec.JobDeleteBefore(ctx, &JobDeleteBeforeParams{
			Max:                1000,
			Schema:             c.schema,
			FinalizedAtHorizon: horizon,
			State:              state,
		})
		if err != nil {
			c.logger.ErrorContext(ctx, "job cleaner error", "state", state, "error", err)
		} else if n > 0 {
			c.logger.DebugContext(ctx, "job cleaner deleted jobs", "count", n, "state", state)
		}
	}
}

type JobRescuer struct {
	exec        Executor
	schema      string
	logger      *slog.Logger
	stopCh      chan struct{}
	rescueAfter time.Duration
}

func NewJobRescuer(exec Executor, schema string, rescueAfter time.Duration) *JobRescuer {
	if rescueAfter <= 0 {
		rescueAfter = time.Hour
	}
	return &JobRescuer{
		exec:        exec,
		schema:      schema,
		logger:      slog.Default(),
		stopCh:      make(chan struct{}),
		rescueAfter: rescueAfter,
	}
}

func (r *JobRescuer) Start(ctx context.Context) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				r.logger.ErrorContext(ctx, "job rescuer panic recovered", "panic", recovered, "stack", string(buf[:n]))
			}
		}()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.stopCh:
				return
			case <-ticker.C:
				r.rescue(ctx)
			}
		}
	}()
}

func (r *JobRescuer) Stop() {
	close(r.stopCh)
}

func (r *JobRescuer) rescue(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-r.rescueAfter)
	jobs, err := r.exec.JobGetStuck(ctx, &JobGetStuckParams{
		Max:               100,
		Schema:            r.schema,
		AttemptedAtBefore: cutoff,
	})
	if err != nil {
		r.logger.ErrorContext(ctx, "job rescuer query failed", "error", err)
		return
	}
	now := time.Now().UTC()
	updates := make([]*JobSetStateIfRunningParams, 0, len(jobs))
	for _, job := range jobs {
		errData, marshalErr := json.Marshal(AttemptError{
			At:      now,
			Attempt: job.Attempt,
			Error:   "job lease expired while running",
		})
		if marshalErr != nil {
			r.logger.ErrorContext(ctx, "job rescuer could not encode error", "job_id", job.ID, "error", marshalErr)
			continue
		}
		update := &JobSetStateIfRunningParams{
			ID:          job.ID,
			Attempt:     &job.Attempt,
			ErrData:     errData,
			ScheduledAt: &now,
			Schema:      r.schema,
			State:       JobStateRetryable,
		}
		if job.Attempt >= job.MaxAttempts {
			update.State = JobStateDiscarded
			update.FinalizedAt = &now
		}
		updates = append(updates, update)
	}
	if len(updates) == 0 {
		return
	}
	if _, err := r.exec.JobSetStateIfRunningMany(ctx, &JobSetStateIfRunningManyParams{Jobs: updates, Schema: r.schema}); err != nil {
		r.logger.ErrorContext(ctx, "job rescuer update failed", "error", err)
		return
	}
	r.logger.InfoContext(ctx, "rescued stuck jobs", "count", len(updates))
}

type JobScheduler struct {
	exec     Executor
	schema   string
	logger   *slog.Logger
	stopCh   chan struct{}
	interval time.Duration
}

func NewJobScheduler(exec Executor, schema string, interval time.Duration) *JobScheduler {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &JobScheduler{
		exec:     exec,
		schema:   schema,
		logger:   slog.Default(),
		stopCh:   make(chan struct{}),
		interval: interval,
	}
}

func (s *JobScheduler) Start(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				s.logger.ErrorContext(ctx, "job scheduler panic recovered", "panic", r, "stack", string(buf[:n]))
			}
		}()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.schedule(ctx)
			}
		}
	}()
}

func (s *JobScheduler) Stop() {
	close(s.stopCh)
}

func (s *JobScheduler) schedule(ctx context.Context) {
	results, err := s.exec.JobSchedule(ctx, &JobScheduleParams{
		Max:    1000,
		Now:    timeNowPtr(),
		Schema: s.schema,
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "job scheduler error", "error", err)
		return
	}

	if len(results) > 0 {
		s.logger.DebugContext(ctx, "job scheduler moved jobs", "count", len(results))
	}
}
