package http

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func adminAuthAllScopes(c *fiber.Ctx) error {
	c.Locals("user", tokenClaims{Sub: "test-user", Role: "admin"})
	c.Locals("apiScopes", []string{
		"nodes.read", "nodes.write", "nodes.delete",
		"servers.read", "servers.write",
		"certificates.read", "certificates.write",
		"settings.read", "settings.write",
		"deployments.read", "deployments.write",
		"traffic.read", "traffic.write",
		"failover.read", "failover.write",
		"autoscaler.read", "autoscaler.write",
		"cloud.read", "cloud.write",
		"reconcile.read", "reconcile.write",
		"backups.read", "backups.write",
		"users.read", "users.write", "users.delete",
		"loadbalancer.read", "loadbalancer.write",
	})
	return c.Next()
}

func TestBackupNameCandidates(t *testing.T) {
	tests := []struct {
		identifier string
		want       []string
	}{
		{identifier: "backup-20260101T000000Z", want: []string{"backup-20260101T000000Z", "backup-20260101T000000Z.zip"}},
		{identifier: "backup-20260101T000000Z.zip", want: []string{"backup-20260101T000000Z.zip", "backup-20260101T000000Z"}},
		{identifier: "123e4567-e89b-12d3-a456-426614174000", want: []string{"123e4567-e89b-12d3-a456-426614174000", "123e4567-e89b-12d3-a456-426614174000.zip"}},
	}
	for _, tt := range tests {
		if got := backupNameCandidates(tt.identifier); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("backupNameCandidates(%q) = %v, want %v", tt.identifier, got, tt.want)
		}
	}
}

func TestServerRoutes_NilStore(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/api/v1", adminAuthAllScopes)
	registerServerRoutes(protected, Config{Store: nil}, nil, nil, noop, noop)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/servers", ""},
		{http.MethodGet, "/api/v1/servers/srv-1", ""},
		{http.MethodPatch, "/api/v1/servers/srv-1", `{"name":"updated"}`},
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
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("expected 503 for nil store, got %d", resp.StatusCode)
			}
		})
	}
}

func TestServerRoutes_NonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "user"})
		return c.Next()
	})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/api/v1")
	registerServerRoutes(protected, Config{}, nil, nil, noop, noop)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v1/users", ""},
		{http.MethodPost, "/api/v1/users", `{}`},
		{http.MethodDelete, "/api/v1/users/user-1", ""},
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
