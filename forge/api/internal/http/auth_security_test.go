package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

type fakeAuthenticationStore struct {
	user       store.User
	userErr    error
	revoked    bool
	revokedErr error
}

func (f *fakeAuthenticationStore) GetUserByID(context.Context, string) (store.User, error) {
	return f.user, f.userErr
}

func (f *fakeAuthenticationStore) IsJWTRevoked(context.Context, string) (bool, error) {
	return f.revoked, f.revokedErr
}

func (f *fakeAuthenticationStore) ValidateApiKey(context.Context, string, string) (*store.User, []string, error) {
	return nil, nil, errors.New("invalid API key")
}

func sessionTokenForTest(t *testing.T, role, email string, version int64) string {
	t.Helper()
	token, err := issueToken("secret", store.User{
		ID: "user-1", Email: email, Role: role, SessionVersion: version,
	})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func requestWithSession(t *testing.T, app *fiber.App, token string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestJWTMiddlewareReloadsCurrentIdentityAndEffectiveRole(t *testing.T) {
	st := &fakeAuthenticationStore{user: store.User{
		ID: "user-1", Email: "current@example.com", Role: "admin", SessionVersion: 4,
	}}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	var got tokenClaims
	app.Get("/resource", authMiddlewareWithStore("secret", st, nil), requireRole("admin"), func(c *fiber.Ctx) error {
		got = c.Locals("user").(tokenClaims)
		return c.SendStatus(http.StatusOK)
	})

	res := requestWithSession(t, app, sessionTokenForTest(t, "user", "stale@example.com", 4))
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("current admin role received %d, want 200", res.StatusCode)
	}
	if got.Email != st.user.Email || got.Role != st.user.Role {
		t.Fatalf("locals = %#v, want current email %q and role %q", got, st.user.Email, st.user.Role)
	}
}

func TestJWTMiddlewareRoleChangesApplyWithoutTokenRefresh(t *testing.T) {
	st := &fakeAuthenticationStore{user: store.User{
		ID: "user-1", Email: "user@example.com", Role: "admin", SessionVersion: 2,
	}}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/resource", authMiddlewareWithStore("secret", st, nil), requireRole("admin"), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})
	token := sessionTokenForTest(t, "user", "user@example.com", 2)

	promoted := requestWithSession(t, app, token)
	promoted.Body.Close()
	if promoted.StatusCode != http.StatusOK {
		t.Fatalf("promoted user received %d, want 200", promoted.StatusCode)
	}

	st.user.Role = "user"
	demoted := requestWithSession(t, app, token)
	defer demoted.Body.Close()
	if demoted.StatusCode != http.StatusForbidden {
		t.Fatalf("demoted user received %d, want 403", demoted.StatusCode)
	}
}

func TestJWTMiddlewareRejectsUnavailableUsersAndStaleSessions(t *testing.T) {
	tests := []struct {
		name string
		st   *fakeAuthenticationStore
	}{
		{name: "deleted", st: &fakeAuthenticationStore{userErr: errors.New("not found")}},
		{name: "disabled", st: &fakeAuthenticationStore{user: store.User{ID: "user-1", Disabled: true, SessionVersion: 3}}},
		{name: "stale revision", st: &fakeAuthenticationStore{user: store.User{ID: "user-1", SessionVersion: 4}}},
		{name: "revoked", st: &fakeAuthenticationStore{revoked: true}},
		{name: "revocation lookup error", st: &fakeAuthenticationStore{revokedErr: errors.New("database unavailable")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			app.Get("/resource", authMiddlewareWithStore("secret", tt.st, nil), func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})
			res := requestWithSession(t, app, sessionTokenForTest(t, "user", "user@example.com", 3))
			defer res.Body.Close()
			if res.StatusCode != http.StatusUnauthorized {
				t.Fatalf("received %d, want 401", res.StatusCode)
			}
		})
	}
}

func TestNormalUserDelegatedScopesCannotReadAdminResources(t *testing.T) {
	tests := []struct {
		name       string
		middleware fiber.Handler
	}{
		{name: "nodes read", middleware: requireAdminScope("nodes.read")},
		{name: "settings read", middleware: requireAdminScope("settings.read")},
		{name: "admin role read", middleware: requireRole("admin")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			app.Get("/resource", func(c *fiber.Ctx) error {
				c.Locals("user", tokenClaims{Sub: "user-1", Role: "user"})
				c.Locals("apiScopes", []string{"servers.read"})
				c.Locals("scopedAuth", true)
				return c.Next()
			}, tt.middleware, func(c *fiber.Ctx) error {
				return c.SendStatus(http.StatusOK)
			})

			res, err := app.Test(httptest.NewRequest(http.MethodGet, "/resource", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusForbidden {
				t.Fatalf("normal user received %d, want 403", res.StatusCode)
			}
		})
	}
}

func TestOAuthServerBindingFailsBeforePermissionLookup(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/servers/:id", func(c *fiber.Ctx) error {
		c.Locals("oauthServerId", "server-a")
		c.Locals("user", tokenClaims{Sub: "user-1", Role: "user"})
		c.Locals("apiScopes", []string{"servers.read"})
		c.Locals("scopedAuth", true)
		return c.Next()
	}, requireServerPermission(Config{}, store.PermWebsocketConnect), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	res, err := app.Test(httptest.NewRequest(http.MethodGet, "/servers/server-b", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("mismatched OAuth server received %d, want 403", res.StatusCode)
	}
}

func TestIssuedSessionTokenContainsRevocationClaims(t *testing.T) {
	user := store.User{
		ID:             "user-1",
		Email:          "current@example.com",
		Role:           "admin",
		SessionVersion: 7,
	}
	token, err := issueToken("secret", user)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := parseToken("secret", token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.JTI == "" {
		t.Fatal("issued token has no JTI")
	}
	if claims.SessionVersion != user.SessionVersion {
		t.Fatalf("session version = %d, want %d", claims.SessionVersion, user.SessionVersion)
	}
}
