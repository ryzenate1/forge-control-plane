//go:build linux

package quota

import (
	"context"
	"errors"

	"golang.org/x/sys/unix"
)

type linuxTracker struct{}

func newTracker() Tracker {
	return &linuxTracker{}
}

func (t *linuxTracker) GetUsage(ctx context.Context, path string) (*Usage, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return nil, err
	}
	total := int64(stat.Blocks) * int64(stat.Bsize)
	free := int64(stat.Bavail) * int64(stat.Bsize)
	used := total - free
	var percent float64
	if total > 0 {
		percent = float64(used) / float64(total) * 100
	}
	return &Usage{
		UsedBytes:   used,
		LimitBytes:  total,
		UsedPercent: percent,
	}, nil
}

func (t *linuxTracker) Enforce(ctx context.Context, path string, currentUsed int64, size int64, limit int64) error {
	if limit <= 0 {
		return nil
	}
	if currentUsed < 0 || size < 0 {
		return errors.New("quota usage and write size must not be negative")
	}
	if currentUsed > limit || size > limit-currentUsed {
		return ErrQuotaExceeded
	}
	return nil
}

func (t *linuxTracker) SetQuota(path string, limitBytes int64) error {
	// Directory/project quotas require filesystem-specific privileged setup
	// (for example XFS project IDs). Returning success here previously claimed
	// an OS quota had been installed when no kernel state was changed.
	return ErrNoQuota
}
