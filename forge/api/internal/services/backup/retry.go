package backup

import (
	"context"
	"crypto/rand"
	"math/big"
	"time"
)

const (
	defaultMaxRetries = 3
	defaultBaseBackoff = 500 * time.Millisecond
	defaultMaxBackoff  = 30 * time.Second
)

type retryConfig struct {
	maxRetries  int
	baseBackoff time.Duration
	maxBackoff  time.Duration
}

func jitterDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	delta := d / 4
	n, err := rand.Int(rand.Reader, big.NewInt(int64(delta*2+1)))
	if err != nil {
		return d
	}
	return d - delta + time.Duration(n.Int64())
}

func withRetry(ctx context.Context, cfg retryConfig, fn func(context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt <= cfg.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := jitterDuration(cfg.baseBackoff * time.Duration(1<<(attempt-1)))
			if backoff > cfg.maxBackoff {
				backoff = cfg.maxBackoff
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}
		if isContextError(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

func isContextError(err error) bool {
	if err == nil {
		return false
	}
	switch err {
	case context.Canceled, context.DeadlineExceeded:
		return true
	}
	return false
}
