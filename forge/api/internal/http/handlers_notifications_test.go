package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestNotificationRoutes_NonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "user"})
		return c.Next()
	})
	protected := app.Group("/api/v1")
	noop := func(c *fiber.Ctx) error { return c.Next() }
	registerNotificationRoutes(protected, nil, noop)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/notification-channels", ""},
		{http.MethodPost, "/api/v1/notification-channels", `{}`},
		{http.MethodGet, "/api/v1/notification-channels/ch-1", ""},
		{http.MethodPatch, "/api/v1/notification-channels/ch-1", `{}`},
		{http.MethodDelete, "/api/v1/notification-channels/ch-1", ""},
		{http.MethodPost, "/api/v1/notification-channels/ch-1/test", ""},
		{http.MethodGet, "/api/v1/notification-channels/ch-1/subscribe", ""},
		{http.MethodPost, "/api/v1/notification-channels/ch-1/subscribe", `{}`},
		{http.MethodDelete, "/api/v1/notification-channels/ch-1/subscribe/sub-1", ""},
		{http.MethodGet, "/api/v1/notification-logs", ""},
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
