package http

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"gamepanel/forge/internal/store"
)

func adminTestMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Sub: "test-admin", Role: "admin"})
		c.Locals("apiScopes", []string{"settings.read", "settings.write"})
		return c.Next()
	}
}

func TestRateLimitSettings_Defaults(t *testing.T) {
	d := store.DefaultRateLimitSettings()
	if d.AuthRequestsPerMinute != 5 {
		t.Fatalf("expected auth 5, got %d", d.AuthRequestsPerMinute)
	}
	if d.MutationRequestsPerMinute != 30 {
		t.Fatalf("expected mutation 30, got %d", d.MutationRequestsPerMinute)
	}
	if d.ReadRequestsPerMinute != 120 {
		t.Fatalf("expected read 120, got %d", d.ReadRequestsPerMinute)
	}
	if d.LoginAttemptThreshold != 5 {
		t.Fatalf("expected login threshold 5, got %d", d.LoginAttemptThreshold)
	}
	if d.AccountLockoutMinutes != 15 {
		t.Fatalf("expected lockout 15, got %d", d.AccountLockoutMinutes)
	}
	if d.SignedURLExpiryMinutes != 5 {
		t.Fatalf("expected signed url expiry 5, got %d", d.SignedURLExpiryMinutes)
	}
	if d.MaxWebSocketsPerServer != 30 {
		t.Fatalf("expected max ws 30, got %d", d.MaxWebSocketsPerServer)
	}
	if d.ConsoleThrottleLines != 2000 {
		t.Fatalf("expected console lines 2000, got %d", d.ConsoleThrottleLines)
	}
	if d.ConsoleThrottlePeriodMs != 100 {
		t.Fatalf("expected console period 100, got %d", d.ConsoleThrottlePeriodMs)
	}
	if !d.LoginRateLimitEnabled {
		t.Fatal("expected login rate limit enabled by default")
	}
	if d.ConsoleThrottleEnabled {
		t.Fatal("expected console throttle disabled by default")
	}
}

func setupRateLimitTestApp(cfg Config) *fiber.App {
	app := fiber.New()
	app.Use(adminTestMiddleware())
	registerRateLimitSettingsRoutes(app, cfg, func(c *fiber.Ctx) error { return c.Next() }, func(c *fiber.Ctx) error { return c.Next() })
	return app
}

func TestRateLimitSettings_NoStore_ReturnsDefaults(t *testing.T) {
	app := setupRateLimitTestApp(Config{})

	req := httptest.NewRequest("GET", "/admin/settings/rate-limits", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var settings store.RateLimitSettings
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.AuthRequestsPerMinute != 5 {
		t.Fatalf("expected auth 5, got %d", settings.AuthRequestsPerMinute)
	}
}

func TestRateLimitSettings_NoStore_UpdateFails(t *testing.T) {
	app := setupRateLimitTestApp(Config{})

	body := `{"authRequestsPerMinute":10,"mutationRequestsPerMinute":60,"readRequestsPerMinute":240,"loginRateLimitEnabled":true,"loginAttemptThreshold":3,"accountLockoutMinutes":30,"signedUrlExpiryMinutes":10,"maxWebSocketsPerServer":50,"consoleThrottleEnabled":true,"consoleThrottleLines":5000,"consoleThrottlePeriodMs":200}`
	req := httptest.NewRequest("PUT", "/admin/settings/rate-limits", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 503 {
		t.Fatalf("expected 503 without store, got %d", resp.StatusCode)
	}
}

func TestRateLimitSettings_InvalidBody(t *testing.T) {
	app := setupRateLimitTestApp(Config{Store: &store.Store{}})

	req := httptest.NewRequest("PUT", "/admin/settings/rate-limits", strings.NewReader("not-json{{{"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for invalid body, got %d", resp.StatusCode)
	}
}
