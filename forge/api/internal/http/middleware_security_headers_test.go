package http

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func bareTestApp() *fiber.App {
	return fiber.New()
}

func TestSecurityHeadersMiddleware_Default(t *testing.T) {
	cfg := DefaultSecurityHeadersConfig()

	app := bareTestApp()
	app.Use(SecurityHeadersMiddleware(cfg))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	headers := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"X-XSS-Protection":        "1; mode=block",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Permissions-Policy":      "geolocation=(), microphone=(), camera=()",
		"Content-Security-Policy": "default-src 'self'",
	}

	for header, expected := range headers {
		got := resp.Header.Get(header)
		if got != expected {
			t.Errorf("header %s: expected %q, got %q", header, expected, got)
		}
	}

	hsts := resp.Header.Get("Strict-Transport-Security")
	if hsts != "max-age=31536000; includeSubDomains" {
		t.Errorf("HSTS: expected max-age=31536000; includeSubDomains, got %q", hsts)
	}
}

func TestSecurityHeadersMiddleware_CustomCSP(t *testing.T) {
	cfg := DefaultSecurityHeadersConfig()
	cfg.CSPValue = "default-src 'self'; script-src 'self' cdn.example.com"

	app := bareTestApp()
	app.Use(SecurityHeadersMiddleware(cfg))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	got := resp.Header.Get("Content-Security-Policy")
	if got != cfg.CSPValue {
		t.Errorf("expected CSP %q, got %q", cfg.CSPValue, got)
	}
}

func TestSecurityHeadersMiddleware_DisableHeaders(t *testing.T) {
	cfg := SecurityHeadersConfig{
		ContentTypeOptions:    false,
		FrameOptions:          false,
		XSSProtection:         false,
		StrictTransport:       false,
		ContentSecurityPolicy: false,
		ReferrerPolicy:        false,
		PermissionsPolicy:     false,
	}

	app := bareTestApp()
	app.Use(SecurityHeadersMiddleware(cfg))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	disabledHeaders := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"X-XSS-Protection",
		"Strict-Transport-Security",
		"Content-Security-Policy",
		"Referrer-Policy",
		"Permissions-Policy",
	}

	for _, h := range disabledHeaders {
		if got := resp.Header.Get(h); got != "" {
			t.Errorf("expected header %s to be absent, got %q", h, got)
		}
	}
}

func TestSecurityHeadersMiddleware_CustomFrameOptions(t *testing.T) {
	cfg := DefaultSecurityHeadersConfig()
	cfg.FrameOptionsValue = "SAMEORIGIN"

	app := bareTestApp()
	app.Use(SecurityHeadersMiddleware(cfg))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	got := resp.Header.Get("X-Frame-Options")
	if got != "SAMEORIGIN" {
		t.Errorf("expected SAMEORIGIN, got %q", got)
	}
}

func TestSecurityHeadersMiddleware_CustomHSTSMaxAge(t *testing.T) {
	cfg := DefaultSecurityHeadersConfig()
	cfg.HSTSMaxAge = 86400

	app := bareTestApp()
	app.Use(SecurityHeadersMiddleware(cfg))
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}

	got := resp.Header.Get("Strict-Transport-Security")
	if got != "max-age=86400; includeSubDomains" {
		t.Errorf("expected max-age=86400; includeSubDomains, got %q", got)
	}
}
