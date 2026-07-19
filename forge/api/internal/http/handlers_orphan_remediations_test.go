package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func TestOrphanRemediationStatusFromRequest(t *testing.T) {
	for _, tt := range []struct {
		path string
		want int
	}{
		{path: "/?status=pending", want: http.StatusOK},
		{path: "/?status=resolved", want: http.StatusOK},
		{path: "/", want: http.StatusOK},
		{path: "/?status=unknown", want: http.StatusBadRequest},
	} {
		t.Run(tt.path, func(t *testing.T) {
			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			app.Get("/", func(c *fiber.Ctx) error {
				_, err := orphanRemediationStatusFromRequest(c)
				if err != nil {
					return err
				}
				return c.SendStatus(fiber.StatusOK)
			})
			response, err := app.Test(httptest.NewRequest(http.MethodGet, tt.path, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tt.want {
				t.Fatalf("status = %d, want %d", response.StatusCode, tt.want)
			}
		})
	}
}

func TestOrphanRemediationRoutesRequireAdmin(t *testing.T) {
	app := orphanRemediationTestApp("user", nil)

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/orphan-remediations/", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
}

func TestOrphanRemediationRoutesRequireResourceScopes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		method string
		path   string
		scopes []string
		want   int
	}{
		{
			name:   "list requires database read in addition to server read",
			method: http.MethodGet,
			path:   "/admin/orphan-remediations/",
			scopes: []string{"servers.read"},
			want:   http.StatusForbidden,
		},
		{
			name:   "list accepts both read scopes",
			method: http.MethodGet,
			path:   "/admin/orphan-remediations/",
			scopes: []string{"servers.read", "databases.read"},
			want:   http.StatusServiceUnavailable,
		},
		{
			name:   "database resolution rejects server delete scope",
			method: http.MethodPost,
			path:   "/admin/orphan-remediations/databases/remediation-1/resolve",
			scopes: []string{"servers.delete"},
			want:   http.StatusForbidden,
		},
		{
			name:   "database resolution accepts database delete scope",
			method: http.MethodPost,
			path:   "/admin/orphan-remediations/databases/remediation-1/resolve",
			scopes: []string{"databases.delete"},
			want:   http.StatusServiceUnavailable,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := orphanRemediationTestApp("admin", tt.scopes)
			response, err := app.Test(httptest.NewRequest(tt.method, tt.path, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tt.want {
				t.Fatalf("status = %d, want %d", response.StatusCode, tt.want)
			}
		})
	}
}

func TestOrphanRemediationResponseJSONNames(t *testing.T) {
	for _, remediation := range []any{
		store.ServerOrphanRemediation{},
		store.DatabaseOrphanRemediation{},
	} {
		body, err := json.Marshal(remediation)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"serverId", "createdAt"} {
			if _, ok := payload[key]; !ok {
				t.Errorf("%T response is missing JSON field %q: %s", remediation, key, body)
			}
		}
		if databaseRemediation, ok := remediation.(store.DatabaseOrphanRemediation); ok {
			for _, key := range []string{"serverDatabaseId", "databaseHostId", "database"} {
				if _, ok := payload[key]; !ok {
					t.Errorf("%T response is missing JSON field %q: %s", databaseRemediation, key, body)
				}
			}
		}
	}
}

func orphanRemediationTestApp(role string, scopes []string) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: role})
		c.Locals("apiScopes", scopes)
		return c.Next()
	})
	registerOrphanRemediationRoutes(app, Config{}, func(c *fiber.Ctx) error { return c.Next() }, func(c *fiber.Ctx) error { return c.Next() })
	return app
}
