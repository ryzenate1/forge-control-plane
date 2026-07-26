package quota

import (
	"context"
	"errors"
)

var (
	ErrQuotaExceeded = errors.New("disk quota exceeded")
	ErrNoQuota       = errors.New("quota tracking not available on this platform")
)

type Usage struct {
	UsedBytes   int64   `json:"usedBytes"`
	LimitBytes  int64   `json:"limitBytes"`
	UsedPercent float64 `json:"usedPercent"`
}

type Tracker interface {
	GetUsage(ctx context.Context, path string) (*Usage, error)

	Enforce(ctx context.Context, path string, currentUsed int64, size int64, limit int64) error

	SetQuota(path string, limitBytes int64) error
}

type NoopTracker struct{}

func (NoopTracker) GetUsage(ctx context.Context, path string) (*Usage, error) {
	return nil, ErrNoQuota
}

func (NoopTracker) Enforce(ctx context.Context, path string, currentUsed int64, size int64, limit int64) error {
	return nil
}

func (NoopTracker) SetQuota(path string, limitBytes int64) error {
	return ErrNoQuota
}

func NewTracker() Tracker {
	return newTracker()
}
