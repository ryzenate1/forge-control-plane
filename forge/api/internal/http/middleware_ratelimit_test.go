package http

import (
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
	if cfg.MaxRequests != 5 {
		t.Fatalf("expected auth max requests 5, got %d", cfg.MaxRequests)
	}
}

func TestExtractClientIPUsesRightmostForwardedAddress(t *testing.T) {
	app := fiber.New()
	app.Get("/ip", func(c *fiber.Ctx) error { return c.SendString(ExtractClientIP(c)) })
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.99, 198.51.100.24")
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 64)
	n, _ := res.Body.Read(body)
	if got := string(body[:n]); got != "198.51.100.24" {
		t.Fatalf("client IP = %q, want rightmost forwarded address", got)
	}
}
