package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestDatabaseHostConnectionTestRoutesRequireWriteScope(t *testing.T) {
	validPayload := `{"name":"primary","engine":"postgresql","host":"db.example.com","port":5432,"username":"panel","password":"secret"}`
	for _, tt := range []struct {
		name   string
		path   string
		scopes []string
		want   int
	}{
		{
			name:   "prospective host test rejects read-only scope",
			path:   "/database-hosts/test",
			scopes: []string{"databases.read"},
			want:   http.StatusForbidden,
		},
		{
			name:   "prospective host test is registered after write authorization",
			path:   "/database-hosts/test",
			scopes: []string{"databases.write"},
			want:   http.StatusServiceUnavailable,
		},
		{
			name:   "saved host test rejects read-only scope",
			path:   "/database-hosts/host-1/test",
			scopes: []string{"databases.read"},
			want:   http.StatusForbidden,
		},
		{
			name:   "saved host test is registered after write authorization",
			path:   "/database-hosts/host-1/test",
			scopes: []string{"databases.write"},
			want:   http.StatusServiceUnavailable,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := databaseHostTestApp(tt.scopes)
			request := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(validPayload))
			request.Header.Set("Content-Type", "application/json")
			response, err := app.Test(request)
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

func databaseHostTestApp(scopes []string) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "admin"})
		c.Locals("apiScopes", scopes)
		return c.Next()
	})
	noop := func(c *fiber.Ctx) error { return c.Next() }
	registerAdminRoutes(app, Config{}, nil, nil, nil, nil, nil, nil, noop, noop)
	return app
}
