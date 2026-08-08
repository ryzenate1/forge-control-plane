package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/go-co-op/gocron"
)

type Scheduler struct {
	store    Store
	adapters map[string]BackupInterface
	cron     *gocron.Scheduler
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	jobs     map[string]*gocron.Job
}

func NewScheduler(store Store, cron *gocron.Scheduler) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		store:    store,
		adapters: make(map[string]BackupInterface),
		cron:     cron,
		ctx:      ctx,
		cancel:   cancel,
		jobs:     make(map[string]*gocron.Job),
	}
}

func (s *Scheduler) RegisterAdapter(name string, adapter BackupInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adapters[name] = adapter
}

func (s *Scheduler) Schedule(serverID, serverRoot, cronExpr string, adapterName string) error {
	if err := validateServerRoot(serverRoot); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	adapter, ok := s.adapters[adapterName]
	if !ok {
		return fmt.Errorf("unknown adapter %q", adapterName)
	}

	if _, exists := s.jobs[serverID]; exists {
		return fmt.Errorf("backup schedule already exists for server %s", serverID)
	}

	job, err := s.cron.Cron(cronExpr).Do(func() {
		ctx, cancel := context.WithTimeout(s.ctx, 6*time.Hour)
		defer cancel()
		backupName := fmt.Sprintf("backup-%d.zip", time.Now().UnixMilli())
		_, err := adapter.Create(ctx, serverID, serverRoot, backupName, nil)
		if err != nil {
			slog.Warn("backup cron job failed", "serverID", serverID, "error", err)
		}
	})
	if err != nil {
		return fmt.Errorf("create cron job: %w", err)
	}

	s.jobs[serverID] = job
	return nil
}

func (s *Scheduler) Close() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	for serverID, job := range s.jobs {
		s.cron.RemoveByReference(job)
		delete(s.jobs, serverID)
	}
}

func (s *Scheduler) Cancel(serverID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, exists := s.jobs[serverID]
	if !exists {
		return fmt.Errorf("no backup schedule for server %s", serverID)
	}

	s.cron.RemoveByReference(job)
	delete(s.jobs, serverID)
	return nil
}

func (s *Scheduler) RunBackup(ctx context.Context, serverID, serverRoot, adapterName string) error {
	if err := validateServerRoot(serverRoot); err != nil {
		return err
	}
	adapter, ok := s.adapters[adapterName]
	if !ok {
		return fmt.Errorf("unknown adapter %q", adapterName)
	}

	backupName := fmt.Sprintf("backup-%d.zip", time.Now().UnixMilli())
	info, err := adapter.Create(ctx, serverID, serverRoot, backupName, nil)
	if err != nil {
		return fmt.Errorf("run backup: %w", err)
	}

	if s.store != nil {
		if err := s.store.Create(ctx, Backup{
			ID:          info.UUID,
			ServerID:    serverID,
			StartedAt:   info.Created,
			CompletedAt: info.CompletedAt,
			Status:      BackupStatusCompleted,
			SizeBytes:   info.Size,
			Adapter:     string(info.Adapter),
		}); err != nil {
			return fmt.Errorf("store backup record: %w", err)
		}
	}

	return nil
}

func validateServerRoot(serverRoot string) error {
	if serverRoot == "" {
		return errors.New("server root is required")
	}
	info, err := os.Stat(serverRoot)
	if err != nil {
		return fmt.Errorf("inspect server root: %w", err)
	}
	if !info.IsDir() {
		return errors.New("server root must be a directory")
	}
	return nil
}

type lastRunInfo struct {
	serverID string
	at       time.Time
	err      error
}
