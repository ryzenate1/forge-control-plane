package http

import (
	"context"
	"fmt"
	"log/slog"
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
	Enabled                bool
	Redis                  *redis.Client
	WindowSeconds          int
	MaxRequests            int
	KeyPrefix              string
	FailClosedOnRedisError bool
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

func init() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("rate limiter cleanup panicked", "panic", r)
			}
		}()
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			globalMemLimiter.cleanup()
		}
	}()
}

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

func (m *memRateLimiter) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for key, b := range m.bkt {
		if now.After(b.expiresAt) {
			delete(m.bkt, key)
		}
	}
}

// ExtractClientIP uses proxy headers only when the direct peer is a local or
// private reverse proxy. It takes the right-most forwarded value so a caller-
// supplied left-most X-Forwarded-For entry cannot rotate rate-limit keys.
func ExtractClientIP(c *fiber.Ctx) string {
	peer := strings.TrimSpace(c.IP())
	peerIP := net.ParseIP(peer)
	if peerIP == nil || !(peerIP.IsLoopback() || peerIP.IsPrivate() || peerIP.IsUnspecified()) {
		return peer
	}
	xff := c.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		for index := len(parts) - 1; index >= 0; index-- {
			candidate := strings.TrimSpace(parts[index])
			if net.ParseIP(candidate) != nil {
				return candidate
			}
		}
	}
	xri := c.Get("X-Real-IP")
	if net.ParseIP(strings.TrimSpace(xri)) != nil {
		return strings.TrimSpace(xri)
	}
	return peer
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

// RateLimiter creates a rate limiting middleware using Redis. Development can
// fall back to memory, but production shared-limit deployments fail closed when
// Redis is unavailable so limits cannot be bypassed per API instance.
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
		key := fmt.Sprintf("%s:ratelimit:%s", cfg.KeyPrefix, clientIP)
		window := time.Duration(cfg.WindowSeconds) * time.Second

		// Try Redis first, fall back to in-memory on any error
		count, err := tryRedis(cfg, key, window)
		if err != nil {
			if cfg.FailClosedOnRedisError {
				return fiber.NewError(fiber.StatusServiceUnavailable, "rate limiter unavailable")
			}
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
	// Do NOT decrement on overflow - just reject. The DECR was causing a TOCTOU race.
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
// Rate limiting is always enabled. Development may use an in-memory fallback,
// while production can require Redis for shared, cross-instance enforcement.
func GetRateLimitForEndpoint(endpointType string, redis *redis.Client, failClosedOnRedisError bool) RateLimitConfig {
	switch endpointType {
	case "auth":
		return RateLimitConfig{
			Enabled:                true,
			Redis:                  redis,
			WindowSeconds:          60,
			MaxRequests:            5,
			KeyPrefix:              "api",
			FailClosedOnRedisError: failClosedOnRedisError,
		}
	case "mutation":
		return RateLimitConfig{
			Enabled:                true,
			Redis:                  redis,
			WindowSeconds:          60,
			MaxRequests:            30,
			KeyPrefix:              "api",
			FailClosedOnRedisError: failClosedOnRedisError,
		}
	case "read":
		return RateLimitConfig{
			Enabled:                true,
			Redis:                  redis,
			WindowSeconds:          60,
			MaxRequests:            120,
			KeyPrefix:              "api",
			FailClosedOnRedisError: failClosedOnRedisError,
		}
	default:
		return RateLimitConfig{
			Enabled:                true,
			Redis:                  redis,
			WindowSeconds:          60,
			MaxRequests:            60,
			KeyPrefix:              "api",
			FailClosedOnRedisError: failClosedOnRedisError,
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
