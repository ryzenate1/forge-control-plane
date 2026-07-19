package http

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// RateLimitConfig defines rate limiting configuration
type RateLimitConfig struct {
	Enabled       bool
	Redis         *redis.Client
	WindowSeconds int
	MaxRequests   int
	KeyPrefix     string
	// TrustedIPs bypass rate limiting entirely
	TrustedIPs []string
}

// in-memory rate limiter bucket for fallback when Redis is unavailable
type memBucket struct {
	count     int
	expiresAt time.Time
}

type memRateLimiter struct {
	mu  sync.Mutex
	bkt map[string]*memBucket
}

var globalMemLimiter = &memRateLimiter{bkt: make(map[string]*memBucket)}

func (m *memRateLimiter) allow(key string, maxRequests int, window time.Duration) (bool, int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	b, ok := m.bkt[key]

	if !ok || now.After(b.expiresAt) {
		m.bkt[key] = &memBucket{count: 1, expiresAt: now.Add(window)}
		return true, maxRequests - 1
	}

	if b.count >= maxRequests {
		return false, 0
	}

	b.count++
	remaining := maxRequests - b.count
	if remaining < 0 {
		remaining = 0
	}
	return true, remaining
}

// ExtractClientIP extracts the real client IP from request headers, respecting
// X-Forwarded-For and X-Real-IP when the app is behind a reverse proxy.
func ExtractClientIP(c *fiber.Ctx) string {
	xff := c.Get("X-Forwarded-For")
	if xff != "" {
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	xri := c.Get("X-Real-IP")
	if xri != "" {
		return strings.TrimSpace(xri)
	}
	return c.IP()
}

func isTrustedIP(clientIP string, trustedIPs []string) bool {
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	for _, entry := range trustedIPs {
		if strings.Contains(entry, "/") {
			_, cidr, err := net.ParseCIDR(entry)
			if err == nil && cidr.Contains(ip) {
				return true
			}
		} else if entry == clientIP {
			return true
		}
	}
	return false
}

// RateLimiter creates a rate limiting middleware using Redis with an in-memory
// fallback so the limiter never fails open when the backing store is unavailable.
func RateLimiter(cfg RateLimitConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip if rate limiting is disabled
		if !cfg.Enabled {
			return c.Next()
		}

		clientIP := ExtractClientIP(c)

		// Bypass rate limiting for trusted IPs
		if len(cfg.TrustedIPs) > 0 && isTrustedIP(clientIP, cfg.TrustedIPs) {
			return c.Next()
		}

		// Build rate limit key based on IP address and path
		key := fmt.Sprintf("%s:ratelimit:%s:%s", cfg.KeyPrefix, clientIP, c.Path())
		window := time.Duration(cfg.WindowSeconds) * time.Second

		// Try Redis first, fall back to in-memory on any error
		count, err := tryRedis(cfg, key, window)
		if err != nil {
			allowed, remaining := globalMemLimiter.allow(key, cfg.MaxRequests, window)
			if !allowed {
				c.Set("Retry-After", strconv.Itoa(cfg.WindowSeconds))
				return fiber.NewError(fiber.StatusTooManyRequests, "rate limit exceeded")
			}
			c.Set("X-RateLimit-Limit", strconv.Itoa(cfg.MaxRequests))
			c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			c.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(window).Unix(), 10))
			return c.Next()
		}

		if count > int64(cfg.MaxRequests) {
			c.Set("Retry-After", strconv.Itoa(cfg.WindowSeconds))
			c.Set("X-RateLimit-Limit", strconv.Itoa(cfg.MaxRequests))
			c.Set("X-RateLimit-Remaining", "0")
			c.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(window).Unix(), 10))
			return fiber.NewError(fiber.StatusTooManyRequests, "rate limit exceeded")
		}

		// Add rate limit headers
		remaining := max(0, cfg.MaxRequests-int(count))
		c.Set("X-RateLimit-Limit", strconv.Itoa(cfg.MaxRequests))
		c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		// Use TTL of the Redis key for Reset if available, otherwise estimate
		reset := time.Now().Add(window).Unix()
		if ttl, ttlErr := getTTL(cfg, key); ttlErr == nil && ttl > 0 {
			reset = time.Now().Add(ttl).Unix()
		}
		c.Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))

		return c.Next()
	}
}

func tryRedis(cfg RateLimitConfig, key string, window time.Duration) (int64, error) {
	if cfg.Redis == nil {
		return 0, fmt.Errorf("redis not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	count, err := cfg.Redis.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		cfg.Redis.Expire(ctx, key, window)
	}
	if count > int64(cfg.MaxRequests) {
		cfg.Redis.Decr(ctx, key)
	}
	return count, nil
}

func getTTL(cfg RateLimitConfig, key string) (time.Duration, error) {
	if cfg.Redis == nil {
		return 0, fmt.Errorf("redis not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	return cfg.Redis.TTL(ctx, key).Result()
}

// GetRateLimitForEndpoint returns appropriate rate limit configuration for different endpoint types.
// Rate limiting is always enabled regardless of Redis availability. When Redis is not configured,
// the in-memory fallback is used instead of disabling rate limiting entirely.
func GetRateLimitForEndpoint(endpointType string, redis *redis.Client) RateLimitConfig {
	switch endpointType {
	case "auth":
		return RateLimitConfig{
			Enabled:       true,
			Redis:         redis,
			WindowSeconds: 60,
			MaxRequests:   5,
			KeyPrefix:     "api",
		}
	case "mutation":
		return RateLimitConfig{
			Enabled:       true,
			Redis:         redis,
			WindowSeconds: 60,
			MaxRequests:   30,
			KeyPrefix:     "api",
		}
	case "read":
		return RateLimitConfig{
			Enabled:       true,
			Redis:         redis,
			WindowSeconds: 60,
			MaxRequests:   120,
			KeyPrefix:     "api",
		}
	default:
		return RateLimitConfig{
			Enabled:       true,
			Redis:         redis,
			WindowSeconds: 60,
			MaxRequests:   60,
			KeyPrefix:     "api",
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
