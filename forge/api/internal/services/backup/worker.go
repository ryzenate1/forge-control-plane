package backup

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"
)

type Worker struct {
	store    *store.Store
	svc      *Service
	daemon   *daemon.Client

	mu       sync.RWMutex
	running  bool
	lastTick time.Time
	lastErr  string
	wg       sync.WaitGroup
}

func NewWorker(store *store.Store, svc *Service, daemon *daemon.Client) *Worker {
	return &Worker{store: store, svc: svc, daemon: daemon}
}

func (w *Worker) Start(ctx context.Context) {
	if w.store == nil || w.svc == nil {
		return
	}
	w.wg.Add(1)
	go func() { defer w.wg.Done(); w.loop(ctx) }()
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

		nextRun, err := w.svc.NextCronRun(policy, now)
		if err != nil {
			w.svc.log("invalid cron for backup policy", "policyId", policy.ID, "serverId", policy.ServerID, "interval", policy.Interval, "error", err)
			continue
		}

		if nextRun.Before(now) || nextRun.Equal(now) {
			if err := w.executePolicy(ctx, policy); err != nil {
				w.recordError(fmt.Errorf("policy %s server %s: %w", policy.ID, policy.ServerID, err))
			}
		}
	}

	w.recordError(nil)
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
	var actorID *string
	pending := store.UpsertBackupRequest{
		Name:   name,
		Status: "pending",
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
		UUID:        entry.UUID,
		Name:        entry.Name,
		Checksum:    entry.Checksum,
		Size:        entry.Size,
		Status:      "completed",
		CompletedAt: &completedAt,
	}, actorID)
	if updateErr != nil {
		return fmt.Errorf("update backup record: %w", updateErr)
	}

	w.svc.log("backup created by policy scheduler", "policyId", policy.ID, "serverId", policy.ServerID, "backupName", entry.Name)

	return nil
}

func (w *Worker) enforceRetentionBeforeBackup(ctx context.Context, policy store.BackupPolicy) error {
	if err := w.svc.EnforceRetentionPolicy(ctx, policy.ServerID, policy); err != nil {
		return err
	}

	cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	deleted, err := w.store.CleanupOldBackupsForServer(cleanupCtx, policy.ServerID, policy.RetentionDays, policy.MaxBackups)
	if err != nil {
		return fmt.Errorf("cleanup old backups: %w", err)
	}
	if deleted > 0 {
		w.svc.log("retention cleanup deleted backups", "serverId", policy.ServerID, "count", deleted)
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
