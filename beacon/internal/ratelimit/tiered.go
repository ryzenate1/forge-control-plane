package ratelimit

import (
	"net/http"
	"strings"
)

type Tier struct {
	Pattern           string
	RequestsPerMinute int
	BurstSize         int
}

func DefaultTiers() []Tier {
	return []Tier{
		{Pattern: "/servers/*/power", RequestsPerMinute: 30, BurstSize: 10},
		{Pattern: "/servers/*/ws/*", RequestsPerMinute: 60, BurstSize: 15},
		{Pattern: "/servers/*/files/*", RequestsPerMinute: 120, BurstSize: 20},
		{Pattern: "*", RequestsPerMinute: 240, BurstSize: 40},
	}
}

type TieredLimiter struct {
	tiers    []tierEntry
	fallback *Limiter
}

type tierEntry struct {
	pattern string
	parts   []string
	limiter *Limiter
}

func NewTieredLimiter(tiers []Tier) *TieredLimiter {
	entries := make([]tierEntry, 0, len(tiers))
	var fallback *Limiter
	for _, t := range tiers {
		limiter := NewLimiter(Config{
			RequestsPerMinute: t.RequestsPerMinute,
			BurstSize:         t.BurstSize,
		})
		if t.Pattern == "*" {
			fallback = limiter
			continue
		}
		parts := strings.Split(strings.Trim(t.Pattern, "/"), "/")
		entries = append(entries, tierEntry{
			pattern: t.Pattern,
			parts:   parts,
			limiter: limiter,
		})
	}
	if fallback == nil {
		fallback = NewLimiter(Config{RequestsPerMinute: 240, BurstSize: 40})
	}
	return &TieredLimiter{tiers: entries, fallback: fallback}
}

func (tl *TieredLimiter) matchLimiter(path string) *Limiter {
	reqParts := strings.Split(strings.Trim(path, "/"), "/")
	for _, entry := range tl.tiers {
		if matchParts(entry.parts, reqParts) {
			return entry.limiter
		}
	}
	return tl.fallback
}

func matchParts(pattern, path []string) bool {
	if len(pattern) != len(path) {
		return false
	}
	for i, p := range pattern {
		if p == "*" {
			continue
		}
		if !strings.EqualFold(p, path[i]) {
			return false
		}
	}
	return true
}

func (tl *TieredLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limiter := tl.matchLimiter(r.URL.Path)
			ip := ExtractIP(r)
			if !limiter.Allow(ip) {
				w.Header().Set("Retry-After", "1")
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"errors":[{"code":"rate_limited","title":"too many requests"}]}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
