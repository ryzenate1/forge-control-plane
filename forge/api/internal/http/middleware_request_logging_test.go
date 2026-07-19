package http

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRequestLoggingMiddleware_LogsRequest(t *testing.T) {
	var logged *RequestLogEntry
	cfg := DefaultRequestLoggingConfig()
	cfg.Logger = func(entry RequestLogEntry) {
		logged = &entry
	}

	app := bareTestApp()
	app.Use(RequestLoggingMiddleware(cfg))
	app.Get("/api/v1/servers", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/api/v1/servers", nil)
	req.Header.Set("User-Agent", "test-agent")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if logged == nil {
		t.Fatal("expected request to be logged")
	}
	if logged.Method != "GET" {
		t.Errorf("expected method GET, got %s", logged.Method)
	}
	if logged.Path != "/api/v1/servers" {
		t.Errorf("expected path /api/v1/servers, got %s", logged.Path)
	}
	if logged.Status != 200 {
		t.Errorf("expected status 200, got %d", logged.Status)
	}
	if logged.UserAgent != "test-agent" {
		t.Errorf("expected user-agent test-agent, got %s", logged.UserAgent)
	}
	if logged.Timestamp == "" {
		t.Error("expected timestamp")
	}
	if logged.Duration == "" {
		t.Error("expected duration")
	}
}

func TestRequestLoggingMiddleware_ExcludesHealthChecks(t *testing.T) {
	var logged *RequestLogEntry
	cfg := DefaultRequestLoggingConfig()
	cfg.Logger = func(entry RequestLogEntry) {
		logged = &entry
	}

	app := bareTestApp()
	app.Use(RequestLoggingMiddleware(cfg))
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if logged != nil {
		t.Fatal("expected health check to be excluded from logging")
	}
}

func TestRequestLoggingMiddleware_ExcludesHealthSubpaths(t *testing.T) {
	var logged *RequestLogEntry
	cfg := DefaultRequestLoggingConfig()
	cfg.Logger = func(entry RequestLogEntry) {
		logged = &entry
	}

	app := bareTestApp()
	app.Use(RequestLoggingMiddleware(cfg))
	app.Get("/health/ready", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/health/ready", nil)
	_, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	if logged != nil {
		t.Fatal("expected health subpath to be excluded from logging")
	}
}

func TestRequestLoggingMiddleware_IncludesHealthWhenConfigured(t *testing.T) {
	var logged *RequestLogEntry
	cfg := DefaultRequestLoggingConfig()
	cfg.ExcludeHealthChecks = false
	cfg.Logger = func(entry RequestLogEntry) {
		logged = &entry
	}

	app := bareTestApp()
	app.Use(RequestLoggingMiddleware(cfg))
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/health", nil)
	_, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	if logged == nil {
		t.Fatal("expected health check to be logged when ExcludeHealthChecks is false")
	}
}

func TestRequestLoggingMiddleware_CapturesRequestID(t *testing.T) {
	var logged *RequestLogEntry
	cfg := DefaultRequestLoggingConfig()
	cfg.Logger = func(entry RequestLogEntry) {
		logged = &entry
	}

	app := bareTestApp()
	app.Use(RequestLoggingMiddleware(cfg))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "req-123")
	_, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	if logged == nil {
		t.Fatal("expected request to be logged")
	}
	if logged.RequestID != "req-123" {
		t.Errorf("expected request ID req-123, got %s", logged.RequestID)
	}
}
