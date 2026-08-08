package queue

import (
	"encoding/json"
	"errors"
	"time"
)

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

func JobStates() []JobState {
	return []JobState{
		JobStateAvailable,
		JobStateCancelled,
		JobStateCompleted,
		JobStateDiscarded,
		JobStatePending,
		JobStateRetryable,
		JobStateRunning,
		JobStateScheduled,
	}
}

type AttemptError struct {
	At      time.Time `json:"at"`
	Attempt int       `json:"attempt"`
	Error   string    `json:"error"`
	Trace   string    `json:"trace"`
}

type JobRow struct {
	ID          int64
	Attempt     int
	AttemptedAt *time.Time
	AttemptedBy []string
	CreatedAt   time.Time
	EncodedArgs []byte
	Errors      []AttemptError
	FinalizedAt *time.Time
	Kind        string
	MaxAttempts int
	Metadata    []byte
	Priority    int
	Queue       string
	ScheduledAt time.Time
	State       JobState
	Tags        []string
	UniqueKey   []byte
	UniqueStates []JobState
}

func (j *JobRow) Output() []byte {
	var metadata struct {
		Output json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(j.Metadata, &metadata); err != nil {
		return nil
	}
	return metadata.Output
}

type Job[T JobArgs] struct {
	*JobRow
	Args T
}

type JobInsertResult struct {
	Job                    *JobRow
	UniqueSkippedAsDuplicate bool
}

var ErrNotFound = errors.New("not found")
var ErrJobRunning = errors.New("running jobs cannot be deleted")
