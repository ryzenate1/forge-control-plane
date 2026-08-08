package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gamepanel/forge/internal/store"
)

// DefaultRetentionPolicy is applied to kinds without an explicit row.
const (
	DefaultRetentionDays = 30
	DefaultRetentionMax  = 8
)

// Retention policies.
type RetentionPolicy = store.BackupRetention

// Retention lists every backup retention policy.
func (s *Service) Retention(ctx context.Context) ([]RetentionPolicy, error) {
	policies, err := s.store.ListRetentionPolicies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list retention policies: %w", err)
	}
	return policies, nil
}

// GetRetention returns one kind's retention policy.
func (s *Service) GetRetention(ctx context.Context, kind string) (*RetentionPolicy, error) {
	policy, err := s.store.GetRetentionPolicy(ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("get retention policy: %w", err)
	}
	return policy, nil
}

// SetRetention upserts a retention policy for a kind.
func (s *Service) SetRetention(ctx context.Context, kind string, retentionDays, retentionMax int, enabled bool) (*RetentionPolicy, error) {
	if retentionDays < 0 || retentionMax < 0 {
		return nil, errors.New("retention values must not be negative")
	}
	policy, err := s.store.SetRetentionPolicy(ctx, kind, retentionDays, retentionMax, enabled)
	if err != nil {
		return nil, fmt.Errorf("set retention policy: %w", err)
	}
	return &policy, nil
}

// runRetentionPass applies every enabled retention policy once: it deletes
// completed managed database backups older than retention_days while always
// keeping the newest retention_max backups per database. Returns the total
// number of backups deleted.
func (s *Service) runRetentionPass(ctx context.Context) (int64, error) {
	policies, err := s.store.ListRetentionPolicies(ctx)
	if err != nil {
		return 0, fmt.Errorf("list retention policies: %w", err)
	}
	var total int64
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}
		until := time.Now().UTC().Add(-time.Duration(policy.RetentionDays) * 24 * time.Hour)
		deleted, deleteErr := s.store.DeleteExpiredManagedBackups(ctx, policy.Kind, until, policy.RetentionMax)
		if deleteErr != nil {
			s.logger.Warn("catalog: retention pass failed for kind",
				slog.String("kind", policy.Kind), slog.String("err", deleteErr.Error()))
			continue
		}
		if deleted > 0 {
			s.logger.Info("catalog: retention deleted expired backups",
				slog.String("kind", policy.Kind), slog.Int64("deleted", deleted))
		}
		total += deleted
	}
	return total, nil
}

// RunRetentionNow executes a single retention pass (used by the HTTP handler
// for on-demand runs).
func (s *Service) RunRetentionNow(ctx context.Context) (int64, error) {
	deleted, err := s.runRetentionPass(ctx)
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

// StartRetentionWorker launches the hourly retention ticker on the provided
// background context. Failures are logged, never fatal.
func (s *Service) StartRetentionWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				s.logger.Info("catalog: retention worker stopped")
				return
			case <-ticker.C:
				passCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
				deleted, err := s.runRetentionPass(passCtx)
				cancel()
				if err != nil {
					s.logger.Warn("catalog: retention pass failed", slog.String("err", err.Error()))
					continue
				}
				if deleted > 0 {
					s.logger.Info("catalog: retention pass completed", slog.Int64("deleted", deleted))
				}
			}
		}
	}()
}
