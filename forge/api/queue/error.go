package queue

import (
	"fmt"
	"time"

	"gamepanel/forge/queue/queuetype"
)

var ErrJobCancelledRemotely = queuetype.ErrJobCancelledRemotely

type JobCancelError = queuetype.JobCancelError

func JobCancel(err error) error {
	return queuetype.JobCancel(err)
}

type JobSnoozeError = queuetype.JobSnoozeError

func JobSnooze(duration time.Duration) error {
	if duration < 0 {
		panic("JobSnooze duration must be >= 0")
	}
	return &queuetype.JobSnoozeError{Duration: duration}
}

type QueueAlreadyAddedError struct {
	Name string
}

func (e *QueueAlreadyAddedError) Error() string {
	return fmt.Sprintf("queue %q already added", e.Name)
}

func (e QueueAlreadyAddedError) Is(target error) bool {
	_, ok := target.(*QueueAlreadyAddedError)
	return ok
}

type QueueNotFoundError struct {
	Name string
}

func (e *QueueNotFoundError) Error() string {
	return fmt.Sprintf("queue %q not found", e.Name)
}

func (e QueueNotFoundError) Is(target error) bool {
	_, ok := target.(*QueueNotFoundError)
	return ok
}

type UnknownJobKindError struct {
	Kind string
}

func (e *UnknownJobKindError) Error() string {
	return fmt.Sprintf("unknown job kind: %s", e.Kind)
}
