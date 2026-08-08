package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"time"
)

type executorResult struct {
	Err             error
	PanicVal        any
	PanicTrace      string
	MetadataUpdates []byte
	NextRetry       time.Time
	SnoozeUntil     *time.Time
	Discard         bool
	Canceled        bool
}

type jobExecutor struct {
	clientRetryPolicy ClientRetryPolicy
	jobRow            *JobRow
	workFunc          func(context.Context, *JobRow) error
	logger            *slog.Logger
}

func (e *jobExecutor) Execute(ctx context.Context) *executorResult {
	res := &executorResult{}

	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			res.PanicVal = r
			res.PanicTrace = string(buf[:n])
			res.Err = fmt.Errorf("panic in job %d: %v", e.jobRow.ID, r)
			e.logger.Error("job panic recovered",
				"job_id", e.jobRow.ID,
				"kind", e.jobRow.Kind,
				"panic", r,
				"stack", res.PanicTrace,
			)
		}
	}()

	err := e.workFunc(ctx, e.jobRow)
	if err != nil {
		res.Err = err

		if errors.Is(err, ErrJobCancelledRemotely) {
			res.Canceled = true
			return res
		}

		var cancelErr *JobCancelError
		if errors.As(err, &cancelErr) {
			res.Canceled = true
			res.Err = cancelErr
			return res
		}

		var snoozeErr *JobSnoozeError
		if errors.As(err, &snoozeErr) {
			until := time.Now().Add(snoozeErr.Duration)
			res.SnoozeUntil = &until
			res.Err = snoozeErr
			return res
		}

		attemptsExhausted := len(e.jobRow.Errors)+1 >= e.jobRow.MaxAttempts
		if e.jobRow.MaxAttempts <= 0 {
			attemptsExhausted = false
		}
		if attemptsExhausted {
			res.Discard = true
		} else {
			res.NextRetry = e.clientRetryPolicy.NextRetry(e.jobRow)
		}
	}

	return res
}

func (e *jobExecutor) reportResult(ctx context.Context, exec Executor, res *executorResult, schema string) {
	switch {
	case res.PanicVal != nil || (res.Err != nil && res.Discard):
		e.reportError(ctx, exec, res, schema)
	case res.Canceled:
		now := time.Now()
		errData, _ := json.Marshal(AttemptError{
			At:      now,
			Attempt: e.jobRow.Attempt,
			Error:   res.Err.Error(),
		})
		err := setJobStateWithRetry(ctx, exec, &JobSetStateIfRunningManyParams{
			Jobs: []*JobSetStateIfRunningParams{
				{
					ID:          e.jobRow.ID,
					Attempt:     &e.jobRow.Attempt,
					State:       JobStateCancelled,
					FinalizedAt: &now,
					ErrData:     errData,
					Schema:      schema,
				},
			},
			Schema: schema,
		})
		if err != nil {
			e.logger.Error("failed to report cancelled job",
				"job_id", e.jobRow.ID, "error", err)
		}
	case res.SnoozeUntil != nil:
		err := setJobStateWithRetry(ctx, exec, &JobSetStateIfRunningManyParams{
			Jobs: []*JobSetStateIfRunningParams{
				{
					ID:          e.jobRow.ID,
					Attempt:     &e.jobRow.Attempt,
					State:       JobStateScheduled,
					ScheduledAt: res.SnoozeUntil,
					Schema:      schema,
				},
			},
			Schema: schema,
		})
		if err != nil {
			e.logger.Error("failed to report snoozed job",
				"job_id", e.jobRow.ID, "error", err)
		}
	case res.Err != nil:
		e.reportError(ctx, exec, res, schema)
	default:
		now := time.Now()
		params := &JobSetStateIfRunningManyParams{
			Jobs: []*JobSetStateIfRunningParams{
				{
					ID:          e.jobRow.ID,
					Attempt:     &e.jobRow.Attempt,
					State:       JobStateCompleted,
					FinalizedAt: &now,
					Schema:      schema,
				},
			},
			Schema: schema,
		}
		if len(res.MetadataUpdates) > 0 {
			params.Jobs[0].MetadataDoMerge = true
			params.Jobs[0].MetadataUpdates = res.MetadataUpdates
		}
		err := setJobStateWithRetry(ctx, exec, params)
		if err != nil {
			e.logger.Error("failed to report completed job",
				"job_id", e.jobRow.ID, "error", err)
		}
	}
}

func (e *jobExecutor) reportError(ctx context.Context, exec Executor, res *executorResult, schema string) {
	now := time.Now()

	errData, _ := json.Marshal(AttemptError{
		At:      now,
		Attempt: e.jobRow.Attempt,
		Error:   res.Err.Error(),
		Trace:   res.PanicTrace,
	})

	if res.Discard {
		err := setJobStateWithRetry(ctx, exec, &JobSetStateIfRunningManyParams{
			Jobs: []*JobSetStateIfRunningParams{
				{
					ID:          e.jobRow.ID,
					Attempt:     &e.jobRow.Attempt,
					State:       JobStateDiscarded,
					FinalizedAt: &now,
					ErrData:     errData,
					Schema:      schema,
				},
			},
			Schema: schema,
		})
		if err != nil {
			e.logger.Error("failed to report discarded job",
				"job_id", e.jobRow.ID, "error", err)
		}
		return
	}

	err := setJobStateWithRetry(ctx, exec, &JobSetStateIfRunningManyParams{
		Jobs: []*JobSetStateIfRunningParams{
			{
				ID:          e.jobRow.ID,
				Attempt:     &e.jobRow.Attempt,
				State:       JobStateRetryable,
				ScheduledAt: &res.NextRetry,
				ErrData:     errData,
				Schema:      schema,
			},
		},
		Schema: schema,
	})
	if err != nil {
		e.logger.Error("failed to report retryable job",
			"job_id", e.jobRow.ID, "error", err)
	}
}

func setJobStateWithRetry(ctx context.Context, exec Executor, params *JobSetStateIfRunningManyParams) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := exec.JobSetStateIfRunningMany(ctx, params); err == nil {
			return nil
		} else {
			lastErr = err
		}
		delay := time.Duration(attempt+1) * 100 * time.Millisecond
		select {
		case <-ctx.Done():
			return errors.Join(lastErr, ctx.Err())
		case <-time.After(delay):
		}
	}
	return lastErr
}
