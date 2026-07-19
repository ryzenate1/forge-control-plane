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
	mu     sync.RWMutex
	limits map[Event]*rate.Limiter
}

func NewLimiterBucket() *LimiterBucket {
	lb := &LimiterBucket{
		limits: make(map[Event]*rate.Limiter),
	}
	lb.limits[AuthenticationEvent] = rate.NewLimiter(rate.Every(5*time.Second), 2)
	lb.limits[SendLogsEvent] = rate.NewLimiter(rate.Every(5*time.Second), 2)
	lb.limits[SendCommandEvent] = rate.NewLimiter(rate.Limit(1), 10)
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

func (lb *LimiterBucket) allowDefault() bool {
	lb.mu.RLock()
	limiter, ok := lb.limits["__default__"]
	lb.mu.RUnlock()
	if ok {
		return limiter.Allow()
	}
	limiter = rate.NewLimiter(rate.Limit(1), 4)
	lb.mu.Lock()
	if existing, ok2 := lb.limits["__default__"]; ok2 {
		limiter = existing
	} else {
		lb.limits["__default__"] = limiter
	}
	lb.mu.Unlock()
	return limiter.Allow()
}
