package queuetype

import (
	"errors"
	"fmt"
	"time"
)

var ErrJobCancelledRemotely = errors.New("job cancelled remotely")
var ErrNotFound = errors.New("not found")

type JobCancelError struct {
	Err error
}

func (e *JobCancelError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("job cancelled: %v", e.Err)
	}
	return "job cancelled"
}

func (e *JobCancelError) Unwrap() error { return e.Err }

func JobCancel(err error) error {
	return &JobCancelError{Err: err}
}

type JobSnoozeError struct {
	Duration time.Duration
}

func (e *JobSnoozeError) Error() string {
	return fmt.Sprintf("job snoozed for %s", e.Duration)
}

type UnknownJobKindError struct {
	Kind string
}

func (e *UnknownJobKindError) Error() string {
	return fmt.Sprintf("unknown job kind: %s", e.Kind)
}

type JobInsertParams struct {
	ID           *int64
	Args         JobArgs
	CreatedAt    *time.Time
	EncodedArgs  []byte
	Kind         string
	MaxAttempts  int
	Metadata     []byte
	Priority     int
	Queue        string
	ScheduledAt  *time.Time
	State        string
	Tags         []string
	UniqueKey    []byte
	UniqueStates byte
}

type JobArgs interface {
	Kind() string
}

type JobState string

const (
	JobStateAvailable JobState = "available"
	JobStateCancelled JobState = "cancelled"
	JobStateCompleted JobState = "completed"
	JobStateDiscarded JobState = "discarded"
	JobStatePending   JobState = "pending"
	JobStateRetryable JobState = "retryable"
	JobStateRunning   JobState = "running"
	JobStateScheduled JobState = "scheduled"
)
