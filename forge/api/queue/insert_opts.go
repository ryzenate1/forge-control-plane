package queue

import "time"

type InsertOpts struct {
	Queue       string
	Priority    int
	MaxAttempts int
	Tags        []string
	ScheduledAt time.Time
	UniqueOpts  *UniqueOpts
	Metadata    map[string]any
}

type UniqueOpts struct {
	ByArgs   bool
	ByPeriod time.Duration
	ByQueue  bool
	ByState  []JobState
	ExcludeKind bool
}

type InsertManyParams struct {
	Args JobArgs
	Opts *InsertOpts
}
