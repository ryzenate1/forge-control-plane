package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestFailoverRoutes_NilService(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/admin", adminAuth)
	registerFailoverRoutes(protected, Config{Store: nil}, nil, noop, noop)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/admin/failover/policies", ""},
		{http.MethodPost, "/admin/failover/policies", `{}`},
		{http.MethodGet, "/admin/failover/policies/pol-1", ""},
		{http.MethodPut, "/admin/failover/policies/pol-1", `{}`},
		{http.MethodDelete, "/admin/failover/policies/pol-1", ""},
		{http.MethodGet, "/admin/failover/policies/node/node-1", ""},
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
				t.Fatalf("expected 404 for nil service, got %d", resp.StatusCode)
			}
		})
	}
}

func TestFailoverRoutes_NonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "user"})
		return c.Next()
	})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/admin")
	registerFailoverRoutes(protected, Config{Store: nil}, nil, noop, noop)

	req := httptest.NewRequest(http.MethodGet, "/admin/failover/policies", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for nil service, got %d", resp.StatusCode)
	}
}
