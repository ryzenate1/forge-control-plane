package http

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func rateLimitTestApp(cfg RateLimitConfig) *fiber.App {
	app := fiber.New()
	app.Use(RateLimiter(cfg))
	app.Get("/limited", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	return app
}

func TestRateLimiterDevFallbackWhenRedisUnavailable(t *testing.T) {
	app := rateLimitTestApp(RateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxRequests:   1,
		KeyPrefix:     "test",
	})

	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected first request to pass via dev memory fallback, got %d", res.StatusCode)
	}
}

func TestRateLimiterFailClosedWhenSharedRedisUnavailable(t *testing.T) {
	app := rateLimitTestApp(RateLimitConfig{
		Enabled:                true,
		WindowSeconds:          60,
		MaxRequests:            1,
		KeyPrefix:              "test",
		FailClosedOnRedisError: true,
	})

	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected shared Redis failure to return 503, got %d", res.StatusCode)
	}
}

func TestGetRateLimitForEndpointCarriesFailClosedFlag(t *testing.T) {
	cfg := GetRateLimitForEndpoint("auth", nil, true)
	if !cfg.FailClosedOnRedisError {
		t.Fatal("expected auth limiter to carry fail-closed flag")
	}
	if cfg.MaxRequests != 20 {
		t.Fatalf("expected auth max requests 20, got %d", cfg.MaxRequests)
	}
}

func TestGetRateLimitForEndpointUsesDistinctKeyPrefixes(t *testing.T) {
	// Every limiter tier shares the key shape "<prefix>:ratelimit:<clientIP>".
	// If two tiers share a prefix they share one Redis counter, so reads burn
	// the login budget (20/min) and mutations inherit read traffic: a user who
	// browses a few pages can no longer log in, and parallel smoke tracks
	// permanently 429 each other on loopback. Distinct prefixes keep each
	// tier's budget independent.
	seen := map[string]string{}
	for _, tier := range []string{"auth", "mutation", "read", "other"} {
		prefix := GetRateLimitForEndpoint(tier, nil, false).KeyPrefix
		if prev, dup := seen[prefix]; dup {
			t.Fatalf("tiers %q and %q share rate-limit key prefix %q", prev, tier, prefix)
		}
		seen[prefix] = tier
	}
}

func TestExtractClientIPUsesRightmostForwardedAddress(t *testing.T) {
	// The trust decision is resolved against an explicit peer rather than through
	// app.Test: Fiber's test harness serves from a synthetic connection, so there
	// is no socket peer for the default-deny check to examine and the header path
	// could never be reached. Passing the peer in keeps this testing the rule its
	// name claims - a trusted proxy's XFF chain yields the right-most address,
	// because the left-most entry is the one the original client wrote.
	t.Setenv("TRUSTED_PROXIES", "127.0.0.1")

	got := resolveClientIP(net.ParseIP("127.0.0.1"), "203.0.113.99, 198.51.100.24", "")
	if got != "198.51.100.24" {
		t.Fatalf("client IP = %q, want rightmost forwarded address", got)
	}

	// A caller-supplied chain from an untrusted peer is ignored outright: the
	// peer's own address is what the limiter keys on.
	if got := resolveClientIP(net.ParseIP("203.0.113.7"), "127.0.0.1, 10.0.0.1", "127.0.0.1"); got != "203.0.113.7" {
		t.Fatalf("untrusted peer must not have headers honoured, got %q", got)
	}

	// Garbage entries fall back to the peer instead of yielding an unparsable key.
	if got := resolveClientIP(net.ParseIP("127.0.0.1"), "unknown, not-an-ip", ""); got != "127.0.0.1" {
		t.Fatalf("unparsable forwarded chain should fall back to the peer, got %q", got)
	}

	// End-to-end wiring: with no trusted proxies configured at all, the header is
	// ignored and the resolved value is the connection peer.
	t.Setenv("TRUSTED_PROXIES", "")
	app := fiber.New()
	app.Get("/ip", func(c *fiber.Ctx) error { return c.SendString(ExtractClientIP(c)) })
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.99")
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 64)
	n, _ := res.Body.Read(body)
	if got := string(body[:n]); got == "203.0.113.99" {
		t.Fatalf("unconfigured TRUSTED_PROXIES must not honour X-Forwarded-For, got %q", got)
	}
}
