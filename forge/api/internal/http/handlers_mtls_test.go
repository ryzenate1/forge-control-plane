package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestMTLSRoutes_NonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "user"})
		return c.Next()
	})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/api/v1")
	registerMTLSRoutes(protected, Config{}, nil, nil, noop, noop)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/mtls/certificates"},
		{http.MethodPost, "/api/v1/mtls/certificates/generate-ca"},
		{http.MethodGet, "/api/v1/mtls/status"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("expected 403 for non-admin, got %d", resp.StatusCode)
			}
		})
	}
}
