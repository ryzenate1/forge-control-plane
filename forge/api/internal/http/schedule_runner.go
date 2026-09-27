package http

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

type scheduleRunner struct {
	cfg      Config
	parser   cron.Parser
	workerID string
	mu       sync.RWMutex
	running  bool
	lastTick time.Time
	lastErr  string
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// lastRetention gates the once-daily metrics-retention sweep. It is only
	// touched from tick (single goroutine), so it needs no lock.
	lastRetention time.Time
}

func newScheduleRunner(cfg Config) *scheduleRunner {
	return &scheduleRunner{
		cfg:      cfg,
		workerID: "schedule-" + uuid.NewString(),
		parser: cron.NewParser(
			cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
		),
	}
}

func (r *scheduleRunner) Start(ctx context.Context) {
	runnerCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				r.cfg.Logger.Error("schedule runner panic recovered", "panic", recovered, "stack", string(buf[:n]))
			}
		}()
		r.loop(runnerCtx)
	}()
}

func (r *scheduleRunner) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

func (r *scheduleRunner) Wait() { r.Stop() }

func (r *scheduleRunner) loop(ctx context.Context) {
	if r.cfg.Store == nil {
		return
	}
	r.mu.Lock()
	r.running = true
	r.mu.Unlock()
	defer func() { r.mu.Lock(); r.running = false; r.mu.Unlock() }()
	fallback := time.NewTicker(time.Minute)
	defer fallback.Stop()

	// Run once quickly at boot so schedules start with next_run_at populated.
	r.tick(ctx)
	events, listenErrs := r.cfg.Store.ListenScheduleEvents(ctx)
	timer := time.NewTimer(r.nextWakeDelay(ctx, time.Now().UTC()))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			r.tick(ctx)
			resetTimer(timer, r.nextWakeDelay(ctx, time.Now().UTC()))
		case err, ok := <-listenErrs:
			if !ok {
				listenErrs = nil
				continue
			}
			if err != nil {
				events = nil
				listenErrs = nil
			}
		case <-timer.C:
			r.tick(ctx)
			resetTimer(timer, r.nextWakeDelay(ctx, time.Now().UTC()))
		case <-fallback.C:
			r.tick(ctx)
			resetTimer(timer, r.nextWakeDelay(ctx, time.Now().UTC()))
		}
	}
}

func (r *scheduleRunner) nextWakeDelay(ctx context.Context, now time.Time) time.Duration {
	if r.cfg.Store == nil {
		return time.Minute
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	nextRunAt, _ := r.cfg.Store.NextScheduleRunAt(lookupCtx, now)
	return scheduleWakeDelay(now, nextRunAt, time.Minute)
}

func scheduleWakeDelay(now time.Time, nextRunAt *time.Time, fallback time.Duration) time.Duration {
	if fallback <= 0 {
		fallback = time.Minute
	}
	if nextRunAt == nil {
		return fallback
	}
	delay := nextRunAt.Sub(now)
	if delay < 0 {
		return 0
	}
	if delay > fallback {
		return fallback
	}
	return delay
}

func resetTimer(timer *time.Timer, delay time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(delay)
}

func (r *scheduleRunner) tick(ctx context.Context) {
	if r.cfg.Store == nil {
		return
	}
	now := time.Now().UTC()

	// Run backup cleanup as part of the tick (every minute)
	// This ensures old backups are cleaned up regularly
	r.runBackupCleanup(ctx)

	// Fail stale backups stuck in pending/running state
	r.runBackupPrune(ctx)

	// Evaluate resource alerts against observed node metrics + heartbeat
	// staleness, and prune time-series retention daily. Without this the alert
	// engine never runs and node_metrics is never bounded.
	r.runAlertEvaluation(ctx)
	r.runMetricsRetention(ctx, now)

	pollCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	schedules, err := r.cfg.Store.ListDueSchedules(pollCtx, now, 64)
	cancel()
	if err != nil {
		r.recordTick(err)
		return
	}
	// Newly created schedules need their first cron occurrence initialized; this
	// update is idempotent and does not execute a run.
	for _, schedule := range schedules {
		if schedule.NextRunAt == nil {
			if next, e := r.nextScheduleRun(schedule, now); e == nil {
				initCtx, initCancel := context.WithTimeout(ctx, 5*time.Second)
				_ = r.cfg.Store.UpdateScheduleRunMeta(initCtx, schedule.ID, nil, &next)
				initCancel()
			}
		}
	}
	for i := 0; i < 64 && ctx.Err() == nil; i++ {
		claimCtx, claimCancel := context.WithTimeout(ctx, 5*time.Second)
		claimed, e := r.cfg.Store.ClaimDueSchedule(claimCtx, now, r.workerID)
		claimCancel()
		if e != nil {
			r.recordTick(e)
			return
		}
		if claimed == nil {
			r.recordTick(nil)
			return
		}
		if e := r.runClaim(ctx, now, *claimed); e != nil {
			r.recordTick(e)
		}
	}
	r.recordTick(nil)
}

// runAlertEvaluation refreshes the notification routes and evaluates heartbeat
// staleness per node. It reuses the schedule runner's tick rather than starting
// a dedicated goroutine, so it inherits the same lifecycle and shutdown drain.
func (r *scheduleRunner) runAlertEvaluation(ctx context.Context) {
	if r.cfg.AlertService == nil || r.cfg.Store == nil {
		return
	}
	evalCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = r.cfg.AlertService.RefreshRoutes(evalCtx)
	nodes, err := r.cfg.Store.ListNodes(evalCtx)
	if err != nil {
		return
	}
	for _, node := range nodes {
		_ = r.cfg.AlertService.CheckStaleHeartbeat(evalCtx, node.ID, node.LastSeenAt, 5*time.Minute)
	}
}

// runMetricsRetention enforces the time-series retention policies once a day.
func (r *scheduleRunner) runMetricsRetention(ctx context.Context, now time.Time) {
	if r.cfg.Observability == nil {
		return
	}
	if !r.lastRetention.IsZero() && now.Sub(r.lastRetention) < 24*time.Hour {
		return
	}
	retCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if _, err := r.cfg.Observability.EnforceRetention(retCtx); err != nil {
		r.cfg.Logger.Error("metrics retention failed", "error", err.Error())
		return
	}
	r.lastRetention = now
}

// runBackupCleanup performs automatic backup cleanup based on retention policies
func (r *scheduleRunner) runBackupCleanup(ctx context.Context) {
	if r.cfg.Store == nil {
		return
	}

	settings, err := r.cfg.Store.GetPanelSettings(ctx)
	if err != nil {
		return
	}

	if !settings.BackupAutoCleanup {
		return
	}

	// A retention sweep is best-effort per backup: it reports how many it
	// actually deleted alongside an error describing whatever failed. Log the
	// failures instead of swallowing them, and keep going — invitation cleanup
	// below is independent work that used to be skipped whenever a single
	// backup deletion failed.
	if r.cfg.BackupSvc != nil {
		deleted, err := r.cfg.BackupSvc.CleanupExpiredBackups(ctx)
		if err != nil {
			r.cfg.Logger.Error("backup retention sweep incomplete", "deleted", deleted, "error", err.Error())
		}
	} else {
		if _, err := r.cfg.Store.CleanupOldBackups(ctx, settings.BackupRetentionDays, settings.BackupAutoCleanup); err != nil {
			r.cfg.Logger.Error("backup retention sweep failed", "error", err.Error())
		}
	}

	if _, err := r.cfg.Store.CleanupExpiredInvitations(ctx); err != nil {
		r.cfg.Logger.Error("expired invitation cleanup failed", "error", err.Error())
	}
}

// runBackupPrune fails backups stuck in pending/running state past the configured threshold
func (r *scheduleRunner) runBackupPrune(ctx context.Context) {
	if r.cfg.Store == nil {
		return
	}

	settings, err := r.cfg.Store.GetPanelSettings(ctx)
	if err != nil {
		return
	}

	pruned, err := r.cfg.Store.FailStaleBackups(ctx, settings.BackupPruneAgeMinutes)
	if err != nil {
		return
	}
	_ = pruned
}

func (r *scheduleRunner) RunNow(ctx context.Context, serverID, scheduleID string) error {
	if r.cfg.Store == nil {
		return fmt.Errorf("store unavailable")
	}
	now := time.Now().UTC()
	claimCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	claimed, err := r.cfg.Store.ClaimManualSchedule(claimCtx, now, r.workerID, serverID, scheduleID)
	cancel()
	if err != nil {
		return err
	}
	if claimed == nil {
		return fmt.Errorf("schedule is offline, already running, or not found")
	}
	return r.runClaim(ctx, now, *claimed)
}

func (r *scheduleRunner) runClaim(ctx context.Context, now time.Time, claimed store.ClaimedSchedule) error {
	schedule := claimed.Schedule
	nextRun, err := r.nextScheduleRun(schedule, now)
	if err != nil {
		r.failClaim(claimed, err.Error())
		return err
	}
	hadFailure, hadSuccess := false, false
	for index, task := range schedule.Tasks {
		if err := r.waitOffset(ctx, claimed, time.Duration(task.TimeOffsetSeconds)*time.Second); err != nil {
			r.failClaim(claimed, "execution canceled: "+err.Error())
			return err
		}
		leaseCtx, leaseCancel := context.WithTimeout(ctx, 5*time.Second)
		err = r.cfg.Store.ExtendScheduleLease(leaseCtx, schedule.ID, claimed.RunID, claimed.WorkerID, time.Now().UTC().Add(store.ScheduleLeaseDuration))
		leaseCancel()
		if err != nil {
			r.failClaim(claimed, err.Error())
			return err
		}
		taskErr := r.executeTask(ctx, schedule.ServerID, task)
		status := store.ScheduleTaskRunSuccess
		var errMsg *string
		if taskErr != nil {
			hadFailure = true
			status = store.ScheduleTaskRunFailed
			message := taskErr.Error()
			errMsg = &message
		} else {
			hadSuccess = true
		}
		recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_ = r.cfg.Store.CreateScheduleTaskRun(recordCtx, claimed.RunID, task.ID, status, errMsg)
		recordCancel()
		if taskErr != nil && !continueAfterTaskFailure(task) {
			for _, skipped := range schedule.Tasks[index+1:] {
				skipCtx, skipCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				message := "skipped after prior task failure"
				_ = r.cfg.Store.CreateScheduleTaskRun(skipCtx, claimed.RunID, skipped.ID, store.ScheduleTaskRunSkipped, &message)
				skipCancel()
			}
			break
		}
	}
	runStatus, runErr := scheduleRunOutcome(hadSuccess, hadFailure)
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	return r.cfg.Store.FinishScheduleClaim(finishCtx, claimed, runStatus, runErr, now, nextRun)
}

func continueAfterTaskFailure(task store.ScheduleTask) bool { return task.ContinueOnFailure }

func scheduleRunOutcome(hadSuccess, hadFailure bool) (store.ScheduleRunStatus, *string) {
	switch {
	case hadFailure && hadSuccess:
		message := "one or more tasks failed"
		return store.ScheduleRunPartial, &message
	case hadFailure:
		message := "execution halted after task failure"
		return store.ScheduleRunFailed, &message
	case !hadSuccess:
		message := "schedule has no runnable tasks"
		return store.ScheduleRunSkipped, &message
	default:
		return store.ScheduleRunSuccess, nil
	}
}

func (r *scheduleRunner) waitOffset(ctx context.Context, claimed store.ClaimedSchedule, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	renew := time.NewTicker(time.Minute)
	defer renew.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		case <-renew.C:
			renewCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := r.cfg.Store.ExtendScheduleLease(renewCtx, claimed.Schedule.ID, claimed.RunID, claimed.WorkerID, time.Now().UTC().Add(store.ScheduleLeaseDuration))
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

func (r *scheduleRunner) failClaim(claimed store.ClaimedSchedule, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = r.cfg.Store.FailScheduleClaim(ctx, claimed, message)
}

func (r *scheduleRunner) recordTick(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastTick = time.Now().UTC()
	r.lastErr = ""
	if err != nil {
		r.lastErr = err.Error()
	}
}

func (r *scheduleRunner) Health() (bool, time.Time, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running, r.lastTick, r.lastErr
}

func (r *scheduleRunner) nextScheduleRun(schedule store.Schedule, from time.Time) (time.Time, error) {
	expr := strings.Join([]string{
		strings.TrimSpace(schedule.CronMinute),
		strings.TrimSpace(schedule.CronHour),
		strings.TrimSpace(schedule.CronDayOfMonth),
		strings.TrimSpace(schedule.CronMonth),
		strings.TrimSpace(schedule.CronDayOfWeek),
	}, " ")
	sch, err := r.parser.Parse(expr)
	if err != nil {
		return time.Time{}, err
	}
	return sch.Next(from), nil
}

func (r *scheduleRunner) executeTask(ctx context.Context, serverID string, task store.ScheduleTask) error {
	action := strings.ToLower(strings.TrimSpace(task.Action))
	switch action {
	case "power":
		signal, _ := task.Payload["signal"].(string)
		signal = strings.ToLower(strings.TrimSpace(signal))
		if signal == "" {
			return fmt.Errorf("power task missing payload.signal")
		}
		if r.cfg.Store == nil || r.cfg.ClusterManager == nil {
			return fmt.Errorf("store and cluster manager unavailable for power task")
		}
		targetCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		// Power signals go through the cluster manager, not straight at the
		// daemon: it is what enforces the suspension guard, records desired
		// state and writes the actual-state transition. A scheduled start on a
		// suspended server must fail, not bypass the flag.
		_, _, err := r.cfg.ClusterManager.RequestServerPower(targetCtx, serverID, signal)
		return err
	case "backup":
		if r.cfg.Daemon == nil || r.cfg.Store == nil {
			return fmt.Errorf("daemon/store unavailable for backup task")
		}
		targetCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		target, err := r.cfg.Store.ServerControlTarget(targetCtx, serverID)
		if err != nil {
			return err
		}
		_, err = r.cfg.Daemon.CreateBackup(targetCtx, target.NodeURL, target.NodeToken, target.ServerID, nil)
		return err
	case "command":
		command, _ := task.Payload["command"].(string)
		command = strings.TrimSpace(command)
		if command == "" {
			return fmt.Errorf("command task missing payload.command")
		}
		if r.cfg.Daemon == nil || r.cfg.Store == nil {
			return fmt.Errorf("daemon/store unavailable for command task")
		}
		targetCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		target, err := r.cfg.Store.ServerControlTarget(targetCtx, serverID)
		if err != nil {
			return err
		}
		return r.cfg.Daemon.SendCommand(targetCtx, target.NodeURL, target.NodeToken, target.ServerID, command)
	default:
		return fmt.Errorf("unsupported task action: %s", action)
	}
}
