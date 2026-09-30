package webhook

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultRate    rate.Limit = 10
	defaultBurst             = 5
	cleanupInterval          = 5 * time.Minute
	cleanupMaxAge            = 10 * time.Minute
)

type limiterEntry struct {
	limiter  *rate.Limiter
	lastUsed time.Time
}

type rateLimiterStore struct {
	mu       sync.Mutex
	limiters map[string]*limiterEntry
	rate     rate.Limit
	burst    int
}

func newRateLimiterStore(r rate.Limit, burst int) *rateLimiterStore {
	return &rateLimiterStore{
		limiters: make(map[string]*limiterEntry),
		rate:     r,
		burst:    burst,
	}
}

func (s *rateLimiterStore) allow(key string) bool {
	s.mu.Lock()
	entry, ok := s.limiters[key]
	if !ok {
		entry = &limiterEntry{limiter: rate.NewLimiter(s.rate, s.burst)}
		s.limiters[key] = entry
	}
	entry.lastUsed = time.Now()
	s.mu.Unlock()
	return entry.limiter.Allow()
}

func (s *rateLimiterStore) cleanup(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for k, v := range s.limiters {
		if v.lastUsed.Before(cutoff) {
			delete(s.limiters, k)
		}
	}
}
