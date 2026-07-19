package api

import (
	"context"
	"errors"
	"testing"
)

type mockRateLimiter struct {
	allow func(ctx context.Context, key string) (bool, error)
}

func (m *mockRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	return m.allow(ctx, key)
}

func TestTieredRateLimiter_UnknownTier(t *testing.T) {
	limiter := &TieredRateLimiter{
		tiers: map[string]RateLimiter{},
	}
	allowed, err := limiter.Allow(context.Background(), "key", "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown tier")
	}
	if allowed {
		t.Error("expected allowed to be false")
	}
}

func TestTieredRateLimiter_Allow(t *testing.T) {
	allowedKey := ""
	mock := &mockRateLimiter{
		allow: func(ctx context.Context, key string) (bool, error) {
			allowedKey = key
			return true, nil
		},
	}
	limiter := &TieredRateLimiter{
		tiers: map[string]RateLimiter{
			"free": mock,
		},
	}
	allowed, err := limiter.Allow(context.Background(), "test-key", "free")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected allowed to be true")
	}
	if allowedKey != "test-key" {
		t.Errorf("expected key 'test-key', got %q", allowedKey)
	}
}

func TestTieredRateLimiter_Allow_Error(t *testing.T) {
	mock := &mockRateLimiter{
		allow: func(ctx context.Context, key string) (bool, error) {
			return false, errors.New("rate limit exceeded")
		},
	}
	limiter := &TieredRateLimiter{
		tiers: map[string]RateLimiter{
			"premium": mock,
		},
	}
	allowed, err := limiter.Allow(context.Background(), "key", "premium")
	if err == nil {
		t.Fatal("expected error")
	}
	if allowed {
		t.Error("expected allowed to be false")
	}
}
