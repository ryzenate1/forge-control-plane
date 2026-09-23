package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestAccountRecoveryRoutes_NilStore(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/api/v1")
	// Production signature is (v1, protected, cfg, authLimiter); the public
	// recovery routes mount on v1, so passing the same group for both keeps
	// every route under /api/v1 for this nil-store 503 check.
	registerAccountRecoveryRoutes(protected, protected, Config{Store: nil}, noop)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/auth/recovery/initiate", `{"email":"test@test.com"}`},
		{http.MethodPost, "/api/v1/auth/recovery/verify", `{"token":"abc","password":"NewPass123!"}`},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, strings.NewReader(ep.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("expected 503 for nil store, got %d", resp.StatusCode)
			}
		})
	}
}
