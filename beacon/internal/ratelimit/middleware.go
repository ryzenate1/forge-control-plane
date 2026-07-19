package ratelimit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Config struct {
	RequestsPerMinute int
	BurstSize         int
	CleanupInterval   time.Duration
}

func (c Config) withDefaults() Config {
	if c.RequestsPerMinute <= 0 {
		c.RequestsPerMinute = 120
	}
	if c.BurstSize <= 0 {
		c.BurstSize = 20
	}
	if c.CleanupInterval <= 0 {
		c.CleanupInterval = time.Minute
	}
	return c
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type Limiter struct {
	mu       sync.RWMutex
	visitors map[string]*visitor
	config   Config
}

func NewLimiter(config Config) *Limiter {
	config = config.withDefaults()
	return &Limiter{
		visitors: make(map[string]*visitor),
		config:   config,
	}
}

func (l *Limiter) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(l.config.CleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				l.cleanup()
			}
		}
	}()
}

func (l *Limiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	threshold := time.Now().Add(-3 * l.config.CleanupInterval)
	for ip, v := range l.visitors {
		if v.lastSeen.Before(threshold) {
			delete(l.visitors, ip)
		}
	}
}

func (l *Limiter) getVisitor(ip string) *visitor {
	l.mu.RLock()
	v, ok := l.visitors[ip]
	l.mu.RUnlock()
	if ok {
		v.lastSeen = time.Now()
		return v
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok = l.visitors[ip]
	if ok {
		v.lastSeen = time.Now()
		return v
	}
	rps := rate.Limit(float64(l.config.RequestsPerMinute) / 60.0)
	limiter := rate.NewLimiter(rps, l.config.BurstSize)
	v = &visitor{limiter: limiter, lastSeen: time.Now()}
	l.visitors[ip] = v
	return v
}

func (l *Limiter) Allow(ip string) bool {
	v := l.getVisitor(ip)
	return v.limiter.Allow()
}

func Middleware(limiter *Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ExtractIP(r)
			if !limiter.Allow(ip) {
				w.Header().Set("Retry-After", fmt.Sprintf("%.0f", 60.0/float64(limiter.config.RequestsPerMinute)))
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(w, `{"errors":[{"code":"rate_limited","title":"too many requests"}]}`)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ExtractIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if idx := strings.IndexByte(forwarded, ','); idx >= 0 {
			return strings.TrimSpace(forwarded[:idx])
		}
		return strings.TrimSpace(forwarded)
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
