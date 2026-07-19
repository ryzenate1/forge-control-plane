package api

import (
	"context"
	"fmt"
	"sync"
)

type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

type TieredRateLimiter struct {
	tiers map[string]RateLimiter
	mu    sync.RWMutex
}

func (t *TieredRateLimiter) Allow(ctx context.Context, key string, tier string) (bool, error) {
	t.mu.RLock()
	limiter, ok := t.tiers[tier]
	t.mu.RUnlock()
	if !ok {
		return false, fmt.Errorf("unknown tier: %s", tier)
	}
	return limiter.Allow(ctx, key)
}
