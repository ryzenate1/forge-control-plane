package websocketlimiter

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Event string

const (
	AuthenticationEvent Event = "auth"
	SetStateEvent       Event = "set state"
	SendLogsEvent       Event = "send logs"
	SendCommandEvent    Event = "send command"
	SendStatsEvent      Event = "send stats"
)

type LimiterBucket struct {
	mu             sync.RWMutex
	limits         map[Event]*rate.Limiter
	defaultOnce    sync.Once
	defaultLimiter *rate.Limiter
}

func NewLimiterBucket() *LimiterBucket {
	lb := &LimiterBucket{
		limits: make(map[Event]*rate.Limiter),
	}
	lb.limits[AuthenticationEvent] = rate.NewLimiter(rate.Every(5*time.Second), 2)
	lb.limits[SetStateEvent] = rate.NewLimiter(rate.Every(time.Second), 4)
	lb.limits[SendLogsEvent] = rate.NewLimiter(rate.Every(5*time.Second), 2)
	lb.limits[SendCommandEvent] = rate.NewLimiter(rate.Limit(1), 10)
	lb.limits[SendStatsEvent] = rate.NewLimiter(rate.Every(time.Second), 4)
	return lb
}

func (lb *LimiterBucket) Allow(event Event) bool {
	lb.mu.RLock()
	limiter, ok := lb.limits[event]
	lb.mu.RUnlock()
	if !ok {
		return lb.allowDefault()
	}
	return limiter.Allow()
}

// allowDefault lazily initializes the shared default limiter exactly once.
// This uses sync.Once instead of a naive double-checked-locking pattern
// (an unsynchronized `if defaultLimiter == nil` read followed by a
// lock-protected re-check) because Go's memory model does not guarantee
// that the unsynchronized read observes a fully-initialized value written
// by another goroutine, which is a data race even though it "usually
// works" in practice.
func (lb *LimiterBucket) allowDefault() bool {
	lb.defaultOnce.Do(func() {
		lb.defaultLimiter = rate.NewLimiter(rate.Limit(1), 4)
	})
	return lb.defaultLimiter.Allow()
}
