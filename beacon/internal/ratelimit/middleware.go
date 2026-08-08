package ratelimit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

type Config struct {
	RequestsPerMinute int
	BurstSize         int
	CleanupInterval   time.Duration
	MaxVisitors       int
	TrustedProxyCIDRs []string
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
	if c.MaxVisitors <= 0 {
		c.MaxVisitors = 10_000
	}
	return c
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen atomic.Int64
}

type Limiter struct {
	mu       sync.RWMutex
	visitors map[string]*visitor
	config   Config
	trusted  []*net.IPNet
}

func NewLimiter(config Config) *Limiter {
	config = config.withDefaults()
	limiter := &Limiter{
		visitors: make(map[string]*visitor),
		config:   config,
	}
	for _, cidr := range config.TrustedProxyCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err == nil {
			limiter.trusted = append(limiter.trusted, network)
		}
	}
	return limiter
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
	// Note: this compares time.Time values (via Before), not raw
	// time.Now().UnixNano() wall-clock integers, so there's no risk of int
	// overflow/truncation or of a manually-subtracted negative duration
	// being misinterpreted here. As a defensive measure against a
	// non-positive CleanupInterval (which should never happen given
	// withDefaults, but could in principle make every visitor look stale
	// if ever mutated directly), clamp it to a sane minimum before use.
	interval := l.config.CleanupInterval
	if interval <= 0 {
		interval = time.Minute
	}
	threshold := time.Now().Add(-3 * interval).UnixNano()
	for ip, v := range l.visitors {
		if v.lastSeen.Load() < threshold {
			delete(l.visitors, ip)
		}
	}
}

func (l *Limiter) getVisitor(ip string) *visitor {
	l.mu.RLock()
	v, ok := l.visitors[ip]
	if ok {
		v.lastSeen.Store(time.Now().UnixNano())
	}
	l.mu.RUnlock()
	if ok {
		return v
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok = l.visitors[ip]
	if ok {
		v.lastSeen.Store(time.Now().UnixNano())
		return v
	}
	if len(l.visitors) >= l.config.MaxVisitors {
		var oldestIP string
		var oldestSeen int64 = time.Now().UnixNano()
		for candidateIP, candidate := range l.visitors {
			if seen := candidate.lastSeen.Load(); seen <= oldestSeen {
				oldestIP, oldestSeen = candidateIP, seen
			}
		}
		if oldestIP != "" {
			delete(l.visitors, oldestIP)
		}
	}
	rps := rate.Limit(float64(l.config.RequestsPerMinute) / 60.0)
	limiter := rate.NewLimiter(rps, l.config.BurstSize)
	v = &visitor{limiter: limiter}
	v.lastSeen.Store(time.Now().UnixNano())
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
			ip := limiter.extractIP(r)
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
	return remoteIP(r.RemoteAddr)
}

func (l *Limiter) extractIP(r *http.Request) string {
	direct := remoteIP(r.RemoteAddr)
	if !l.isTrustedProxy(net.ParseIP(direct)) {
		return direct
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		// Walk right-to-left and return the first address that is not one of
		// our trusted proxies. This prevents clients from choosing an
		// arbitrary left-most X-Forwarded-For value.
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])
			ip := net.ParseIP(candidate)
			if ip != nil && !l.isTrustedProxy(ip) {
				return ip.String()
			}
		}
	}
	if realIP := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); realIP != nil {
		return realIP.String()
	}
	return direct
}

func (l *Limiter) isTrustedProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, network := range l.trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}
