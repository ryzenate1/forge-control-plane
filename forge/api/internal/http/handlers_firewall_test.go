package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestFirewallRoutes_NonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "user"})
		return c.Next()
	})
	protected := app.Group("/api/v1")
	noop := func(c *fiber.Ctx) error { return c.Next() }
	registerFirewallRoutes(protected, Config{Store: nil}, noop)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/host/firewall/status", ""},
		{http.MethodPost, "/api/v1/host/firewall/enable", ""},
		{http.MethodPost, "/api/v1/host/firewall/disable", ""},
		{http.MethodGet, "/api/v1/host/firewall/rules", ""},
		{http.MethodPost, "/api/v1/host/firewall/rules", `{}`},
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
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("expected 403 for non-admin, got %d", resp.StatusCode)
			}
		})
	}
}
