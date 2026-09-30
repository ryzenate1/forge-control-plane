// Package scheduledtasks implements per-app scheduled tasks: cron-expression
// commands that run inside an application server's container context. It is
// inspired by Dokku's cron plugin (declare `cron report` entries next to the
// app) and Dokploy's schedule router (API-managed cron jobs whose execution is
// dispatched to the agent), adapted to Forge's API -> Beacon split: the API
// owns scheduling state, Beacon only executes what it is told.
package scheduledtasks

import "gamepanel/forge/internal/store"

// ScheduledTask is one cron entry bound to a server. Schedule is a standard
// 5-field cron expression (minute hour day-of-month month day-of-week).
type ScheduledTask = store.ScheduledTask

// TaskRun is the recorded result of a single dispatch (scheduled or manual).
type TaskRun = store.TaskRun

// CreateInput carries the fields required to register a new task. Enabled
// defaults to true when nil.
type CreateInput struct {
	ServerID  string
	Name      string
	Command   string
	Schedule  string
	Enabled   *bool
	CreatedBy string
}

// UpdateInput carries patch semantics for the editable fields. NextRunAt is
// recomputed by the service whenever the schedule or enabled flag changes,
// so callers do not set it directly.
type UpdateInput struct {
	Name     *string
	Command  *string
	Schedule *string
	Enabled  *bool
}
