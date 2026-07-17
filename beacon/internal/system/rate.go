package system

import (
	"sync"
	"time"
)

// Rate defines a rate limiter of n items (limit) per duration of time.
type Rate struct {
	mu       sync.Mutex
	limit    uint64
	duration time.Duration
	count    uint64
	last     time.Time
}

// NewRate returns a new rate limiter.
func NewRate(limit uint64, duration time.Duration) *Rate {
	return &Rate{
		limit:    limit,
		duration: duration,
		last:     time.Now(),
	}
}

// Try returns true if under the rate limit defined, or false if it has been exceeded.
func (r *Rate) Try() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if now.Sub(r.last) > r.duration {
		r.count = 0
		r.last = now
	}
	if (r.count + 1) > r.limit {
		return false
	}
	r.count++
	return true
}

// Reset resets the internal state of the rate limiter back to zero.
func (r *Rate) Reset() {
	r.mu.Lock()
	r.count = 0
	r.last = time.Now()
	r.mu.Unlock()
}
