package store

import (
	"time"
)

type ScheduleTaskAction string

type Schedule struct {
	ID             string         `json:"id"`
	ServerID       string         `json:"serverId"`
	Name           string         `json:"name"`
	CronExpression string         `json:"cronExpression"`
	CronMinute     string         `json:"cronMinute"`
	CronHour       string         `json:"cronHour"`
	CronDayOfMonth string         `json:"cronDayOfMonth"`
	CronMonth      string         `json:"cronMonth"`
	CronDayOfWeek  string         `json:"cronDayOfWeek"`
	Timezone       string         `json:"timezone"`
	OnlyWhenOnline bool           `json:"onlyWhenOnline"`
	Enabled        bool           `json:"enabled"`
	Active         bool           `json:"active"`
	LastRunAt      *time.Time     `json:"lastRunAt,omitempty"`
	NextRunAt      *time.Time     `json:"nextRunAt,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	Tasks          []ScheduleTask `json:"tasks"`
}

type ScheduleTask struct {
	ID                string         `json:"id"`
	ScheduleID        string         `json:"scheduleId"`
	Sequence          int            `json:"sequence"`
	Action            string         `json:"action"`
	Payload           map[string]any `json:"payload"`
	TimeOffsetSeconds int            `json:"timeOffsetSeconds"`
	ContinueOnFailure bool           `json:"continueOnFailure"`
	CreatedAt         time.Time      `json:"createdAt"`
}

type CreateScheduleRequest struct {
	Name           string
	CronMinute     string
	CronHour       string
	CronDayOfMonth string
	CronMonth      string
	CronDayOfWeek  string
	Timezone       string
	OnlyWhenOnline bool
	Enabled        bool
}

type PatchScheduleRequest struct {
	Name           *string
	CronMinute     *string
	CronHour       *string
	CronDayOfMonth *string
	CronMonth      *string
	CronDayOfWeek  *string
	Timezone       *string
	OnlyWhenOnline *bool
	Enabled        *bool
}

type CreateScheduleTaskRequest struct {
	Sequence          int
	Action            string
	Payload           map[string]any
	TimeOffsetSeconds int
	ContinueOnFailure bool
}

type PatchScheduleTaskRequest struct {
	Sequence          *int
	Action            *string
	Payload           *map[string]any
	TimeOffsetSeconds *int
	ContinueOnFailure *bool
}

type ScheduleRunStatus string

type ScheduleTaskRunStatus string

type ScheduleRun struct {
	ID         string            `json:"id"`
	ScheduleID string            `json:"scheduleId"`
	ServerID   string            `json:"serverId"`
	Status     ScheduleRunStatus `json:"status"`
	Trigger    string            `json:"trigger"`
	Error      *string           `json:"error,omitempty"`
	StartedAt  time.Time         `json:"startedAt"`
	FinishedAt *time.Time        `json:"finishedAt,omitempty"`
	Tasks      []ScheduleTaskRun `json:"tasks"`
}

type ScheduleTaskRun struct {
	ID             string                `json:"id"`
	ScheduleRunID  string                `json:"scheduleRunId"`
	ScheduleTaskID string                `json:"scheduleTaskId"`
	Status         ScheduleTaskRunStatus `json:"status"`
	Error          *string               `json:"error,omitempty"`
	ExecutedAt     time.Time             `json:"executedAt"`
}
