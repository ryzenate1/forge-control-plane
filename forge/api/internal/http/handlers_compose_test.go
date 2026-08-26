package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestComposeRoutes_NilStore(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/api/v1", adminAuth)
	registerComposeRoutes(protected, Config{Store: nil}, noop)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/compose/validate", `{}`},
		{http.MethodPost, "/api/v1/compose/import", `{}`},
		{http.MethodPost, "/api/v1/compose/git/deploy", `{}`},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var req *http.Request
			if ep.body != "" {
				req = httptest.NewRequest(ep.method, ep.path, strings.NewReader(ep.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(ep.method, ep.path, nil)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("expected 404 for nil store, got %d", resp.StatusCode)
			}
		})
	}
}

func TestComposeRoutes_NonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "user"})
		return c.Next()
	})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/api/v1")
	registerComposeRoutes(protected, Config{Store: nil}, noop)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/compose/validate", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for nil store, got %d", resp.StatusCode)
	}
}
