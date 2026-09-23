package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestSocialAuthRoutes_NilStore(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	protected := app.Group("/api/v1")
	// Production signature is (v1, protected, cfg, mutationLimiter, authLimiters...);
	// mounting both halves on the same group keeps every route under /api/v1.
	registerSocialAuthRoutes(protected, protected, Config{Store: nil}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/social/google", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for nil store, got %d", resp.StatusCode)
	}
}
