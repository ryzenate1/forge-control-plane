package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func settingsTestApp(user tokenClaims, scopes []string, st *store.Store) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", user)
		if scopes != nil {
			c.Locals("apiScopes", scopes)
		}
		return c.Next()
	})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	protected := app.Group("/api/v1")
	registerSettingsRoutes(protected, Config{Store: st}, noop, noop)
	return app
}

func settingsRequest(t *testing.T, app *fiber.App, method, path, body string) (*http.Response, string) {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(raw)
}

func TestSettingsRoutes_NonAdmin(t *testing.T) {
	app := settingsTestApp(tokenClaims{Role: "user"}, []string{"settings.write"}, nil)

	resp, body := settingsRequest(t, app, http.MethodPut, "/api/v1/admin/settings", `{}`)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "insufficient role") {
		t.Fatalf("expected role rejection error, got: %s", body)
	}
}

func TestSettingsRoutes_MissingScopeContext(t *testing.T) {
	app := settingsTestApp(tokenClaims{Role: "admin"}, nil, nil)

	resp, body := settingsRequest(t, app, http.MethodGet, "/api/v1/admin/settings", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without api-scope context, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "missing api scope context") {
		t.Fatalf("expected missing scope error, got: %s", body)
	}
}

func TestSettingsRoutes_AdminGetDefaultsWithoutStore(t *testing.T) {
	app := settingsTestApp(tokenClaims{Role: "admin"}, []string{"settings.read"}, nil)

	resp, body := settingsRequest(t, app, http.MethodGet, "/api/v1/admin/settings", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Forge Control Plane") {
		t.Fatalf("expected default settings payload, got: %s", body)
	}
}

func TestSettingsRoutes_PutWithoutStore(t *testing.T) {
	app := settingsTestApp(tokenClaims{Role: "admin"}, []string{"settings.write"}, nil)

	resp, body := settingsRequest(t, app, http.MethodPut, "/api/v1/admin/settings", `{}`)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with nil store, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "postgres is required") {
		t.Fatalf("expected postgres-required error, got: %s", body)
	}
}
