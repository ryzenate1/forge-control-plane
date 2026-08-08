package backup

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"
)

type Worker struct {
	store  *store.Store
	svc    *Service
	daemon *daemon.Client
	jobSvc *JobService

	mu       sync.RWMutex
	running  bool
	lastTick time.Time
	lastErr  string
	wg       sync.WaitGroup
	stopCh   chan struct{}
}

func NewWorker(store *store.Store, svc *Service, daemon *daemon.Client) *Worker {
	return &Worker{store: store, svc: svc, daemon: daemon, stopCh: make(chan struct{})}
}

// SetJobService enables the worker to pick up pending/failed backup jobs and
// re-execute them with backoff.
func (w *Worker) SetJobService(jobSvc *JobService) {
	w.jobSvc = jobSvc
}

func (w *Worker) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.running {
		return
	}
	w.running = false
	select {
	case <-w.stopCh:
		// already closed
	default:
		close(w.stopCh)
	}
}

func (w *Worker) Start(ctx context.Context) {
	if w.store == nil || w.svc == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return
	}
	w.stopCh = make(chan struct{})
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				w.svc.log("backup worker panic recovered", "panic", r, "stack", string(buf[:n]))
			}
		}()
		w.loop(ctx)
	}()
}

func (w *Worker) Wait() { w.wg.Wait() }

func (w *Worker) Health() (bool, time.Time, string) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running, w.lastTick, w.lastErr
}

func (w *Worker) loop(ctx context.Context) {
	w.mu.Lock()
	w.running = true
	w.mu.Unlock()
	defer func() { w.mu.Lock(); w.running = false; w.mu.Unlock() }()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	if w.store == nil || w.svc == nil {
		return
	}

	now := time.Now().UTC()
	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	policies, err := w.svc.ListAllEnabledPolicies(pollCtx)
	if err != nil {
		w.recordError(err)
		return
	}

	for _, policy := range policies {
		if policy.Interval == "" {
			continue
		}

		if !policyDue(policy, now) {
			continue
		}

		if err := w.executePolicy(ctx, policy); err != nil {
			w.recordError(fmt.Errorf("policy %s server %s: %w", policy.ID, policy.ServerID, err))
		}

		if err := w.persistNextRun(ctx, policy, now); err != nil {
			w.svc.log("persist next run for backup policy", "policyId", policy.ID, "serverId", policy.ServerID, "error", err)
		}
	}

	w.pickupBackupJobs(ctx)

	w.recordError(nil)
}

// policyDue reports whether a backup policy is due to run at the given time.
// The policy's persisted NextRunAt (set by persistNextRun) is the source of
// truth; robfig/cron Schedule.Next always returns a strictly-greater time, so
// the naive "nextRun <= now" comparison can never fire.
func policyDue(policy store.BackupPolicy, now time.Time) bool {
	if policy.Interval == "" {
		return false
	}
	if policy.NextRunAt == nil {
		// First tick for this policy: let persistNextRun initialize the
		// schedule without executing immediately.
		return false
	}
	return !policy.NextRunAt.After(now)
}

// persistNextRun computes the next cron occurrence after now (or after the
// policy's current next run when it is still in the future) and stores it.
func (w *Worker) persistNextRun(ctx context.Context, policy store.BackupPolicy, now time.Time) error {
	from := now
	if policy.NextRunAt != nil && policy.NextRunAt.After(now) {
		from = *policy.NextRunAt
	}
	nextRun, err := w.svc.NextCronRun(policy, from)
	if err != nil {
		return err
	}
	return w.store.UpdateBackupPolicyNextRun(ctx, policy.ID, &nextRun)
}

// pickupBackupJobs re-executes backup jobs that are pending or failed with
// retries remaining, honoring a per-attempt backoff. Execution is guarded by
// an atomic status transition in the store, so concurrent workers cannot run
// the same job twice.
func (w *Worker) pickupBackupJobs(ctx context.Context) {
	if w.jobSvc == nil || w.store == nil {
		return
	}
	jobs, err := w.store.ListRetryableBackupJobs(ctx, 5)
	if err != nil {
		w.svc.log("list retryable backup jobs", "error", err)
		return
	}
	for _, job := range jobs {
		runCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
		err := w.jobSvc.Execute(runCtx, job.ID, "scheduler")
		cancel()
		if err != nil {
			w.svc.log("re-execute backup job", "jobId", job.ID, "error", err)
		}
	}
}

func (w *Worker) executePolicy(ctx context.Context, policy store.BackupPolicy) error {
	if err := w.enforceRetentionBeforeBackup(ctx, policy); err != nil {
		w.svc.log("retention enforcement failed before backup", "policyId", policy.ID, "serverId", policy.ServerID, "error", err)
	}

	target, err := w.store.ServerControlTarget(ctx, policy.ServerID)
	if err != nil {
		return fmt.Errorf("lookup server control target: %w", err)
	}

	name := fmt.Sprintf("backup-%s", time.Now().UTC().Format("20060102T150405Z"))

	if policy.VolumeBackup {
		return w.executeVolumeBackup(ctx, policy, target, name)
	}
	if policy.DatabaseType != "" {
		return w.executeDatabaseBackup(ctx, policy, target, name)
	}
	return w.executeServerBackup(ctx, policy, target, name)
}

func (w *Worker) executeServerBackup(ctx context.Context, policy store.BackupPolicy, target store.ServerControlTarget, name string) error {
	var actorID *string
	pending := store.UpsertBackupRequest{
		Name:       name,
		Status:     "pending",
		SourceType: "server",
	}
	stored, storeErr := w.store.UpsertBackup(ctx, target.ServerID, pending, actorID)
	if storeErr != nil {
		return fmt.Errorf("create pending backup record: %w", storeErr)
	}

	backupCtx, backupCancel := context.WithTimeout(ctx, 15*time.Minute)
	defer backupCancel()

	entry, daemonErr := w.daemon.CreateBackup(backupCtx, target.NodeURL, target.NodeToken, target.ServerID, nil)
	if daemonErr != nil {
		now := time.Now().UTC()
		_, _ = w.store.UpsertBackup(ctx, target.ServerID, store.UpsertBackupRequest{
			UUID:        stored.UUID,
			Name:        stored.Name,
			Status:      "failed",
			CompletedAt: &now,
		}, actorID)
		return fmt.Errorf("daemon backup create: %w", daemonErr)
	}

	completedAt := time.Now().UTC()
	if entry.Completed != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, entry.Completed); parseErr == nil {
			completedAt = parsed
		}
	}

	_, updateErr := w.store.UpsertBackup(ctx, target.ServerID, store.UpsertBackupRequest{
		UUID:             entry.UUID,
		Name:             entry.Name,
		Checksum:         entry.Checksum,
		Size:             entry.Size,
		Status:           "completed",
		CompletedAt:      &completedAt,
		SourceType:       "server",
		ChecksumVerified: entry.Checksum != "",
		Compressed:       policy.Compress,
	}, actorID)
	if updateErr != nil {
		return fmt.Errorf("update backup record: %w", updateErr)
	}

	w.svc.log("backup created by policy scheduler", "policyId", policy.ID, "serverId", policy.ServerID, "backupName", entry.Name)
	return nil
}

func (w *Worker) executeDatabaseBackup(ctx context.Context, policy store.BackupPolicy, target store.ServerControlTarget, name string) error {
	var actorID *string

	dbTarget, dbErr := w.store.GetDBContainerBackupTarget(ctx, policy.DatabaseID)
	if dbErr != nil {
		return fmt.Errorf("lookup db container %s: %w", policy.DatabaseID, dbErr)
	}

	pending := store.UpsertBackupRequest{
		Name:         name,
		Status:       "pending",
		SourceType:   "database",
		SourceID:     policy.DatabaseID,
		DatabaseType: policy.DatabaseType,
	}
	stored, storeErr := w.store.UpsertBackup(ctx, target.ServerID, pending, actorID)
	if storeErr != nil {
		return fmt.Errorf("create pending db backup record: %w", storeErr)
	}
	_ = stored

	backupCtx, backupCancel := context.WithTimeout(ctx, 15*time.Minute)
	defer backupCancel()

	entry, daemonErr := w.daemon.BackupDatabase(backupCtx, target.NodeURL, target.NodeToken, dbTarget.ContainerID, dbTarget.Engine, stored.UUID)
	if daemonErr != nil {
		now := time.Now().UTC()
		_, _ = w.store.UpsertBackup(ctx, target.ServerID, store.UpsertBackupRequest{
			UUID:        stored.UUID,
			Name:        stored.Name,
			Status:      "failed",
			CompletedAt: &now,
		}, actorID)
		return fmt.Errorf("daemon database backup: %w", daemonErr)
	}

	completedAt := time.Now().UTC()
	if entry.Completed != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, entry.Completed); parseErr == nil {
			completedAt = parsed
		}
	}

	_, updateErr := w.store.UpsertBackup(ctx, target.ServerID, store.UpsertBackupRequest{
		UUID:             entry.UUID,
		Name:             entry.Name,
		Checksum:         entry.Checksum,
		Size:             entry.Size,
		Status:           "completed",
		CompletedAt:      &completedAt,
		SourceType:       "database",
		SourceID:         policy.DatabaseID,
		DatabaseType:     policy.DatabaseType,
		ChecksumVerified: entry.Checksum != "",
		Compressed:       policy.Compress,
	}, actorID)
	if updateErr != nil {
		return fmt.Errorf("update db backup record: %w", updateErr)
	}

	w.svc.log("database backup created by policy scheduler", "policyId", policy.ID, "serverId", policy.ServerID, "backupName", entry.Name, "dbType", policy.DatabaseType)
	return nil
}

func (w *Worker) executeVolumeBackup(ctx context.Context, policy store.BackupPolicy, target store.ServerControlTarget, name string) error {
	var actorID *string

	volumeName := policy.DatabaseID
	if volumeName == "" {
		volumeName = name
	}

	pending := store.UpsertBackupRequest{
		Name:       name,
		Status:     "pending",
		SourceType: "volume",
		VolumeName: volumeName,
	}
	stored, storeErr := w.store.UpsertBackup(ctx, target.ServerID, pending, actorID)
	if storeErr != nil {
		return fmt.Errorf("create pending volume backup record: %w", storeErr)
	}
	_ = stored

	backupCtx, backupCancel := context.WithTimeout(ctx, 15*time.Minute)
	defer backupCancel()

	entry, daemonErr := w.daemon.BackupVolume(backupCtx, target.NodeURL, target.NodeToken, target.ServerID, volumeName)
	if daemonErr != nil {
		now := time.Now().UTC()
		_, _ = w.store.UpsertBackup(ctx, target.ServerID, store.UpsertBackupRequest{
			UUID:        stored.UUID,
			Name:        stored.Name,
			Status:      "failed",
			CompletedAt: &now,
		}, actorID)
		return fmt.Errorf("daemon volume backup: %w", daemonErr)
	}

	completedAt := time.Now().UTC()
	if entry.Completed != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, entry.Completed); parseErr == nil {
			completedAt = parsed
		}
	}

	_, updateErr := w.store.UpsertBackup(ctx, target.ServerID, store.UpsertBackupRequest{
		UUID:             entry.UUID,
		Name:             entry.Name,
		Checksum:         entry.Checksum,
		Size:             entry.Size,
		Status:           "completed",
		CompletedAt:      &completedAt,
		SourceType:       "volume",
		VolumeName:       volumeName,
		ChecksumVerified: entry.Checksum != "",
		Compressed:       policy.Compress,
	}, actorID)
	if updateErr != nil {
		return fmt.Errorf("update volume backup record: %w", updateErr)
	}

	w.svc.log("volume backup created by policy scheduler", "policyId", policy.ID, "serverId", policy.ServerID, "backupName", entry.Name)
	return nil
}

func (w *Worker) enforceRetentionBeforeBackup(ctx context.Context, policy store.BackupPolicy) error {
	if err := w.svc.EnforceRetentionPolicy(ctx, policy.ServerID, policy); err != nil {
		return err
	}

	cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	backups, listErr := w.store.ListBackups(cleanupCtx, policy.ServerID, 1, 1000)
	if listErr != nil {
		return fmt.Errorf("list backups for cleanup: %w", listErr)
	}

	var countCompleted int
	for _, b := range backups {
		if b.Status == "completed" && !b.IsLocked {
			countCompleted++
		}
	}

	overLimit := 0
	if policy.MaxBackups > 0 && countCompleted >= policy.MaxBackups {
		overLimit = countCompleted - policy.MaxBackups + 1
	}

	if overLimit > 0 {
		var toDelete []store.Backup
		for _, b := range backups {
			if b.Status == "completed" && !b.IsLocked {
				toDelete = append(toDelete, b)
			}
		}
		sort.SliceStable(toDelete, func(i, j int) bool {
			return toDelete[i].CreatedAt.Before(toDelete[j].CreatedAt)
		})
		for i := 0; i < overLimit && i < len(toDelete); i++ {
			b := toDelete[i]
			if delErr := w.svc.DeleteBackupFromStorage(cleanupCtx, b.ServerID, b.Name, policy.Storage); delErr != nil {
				continue
			}
			if dbErr := w.store.DeleteBackup(cleanupCtx, b.ServerID, b.Name, nil); dbErr != nil {
				continue
			}
		}
	}

	return nil
}

func (w *Worker) recordError(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastTick = time.Now().UTC()
	w.lastErr = ""
	if err != nil {
		w.lastErr = err.Error()
	}
}
