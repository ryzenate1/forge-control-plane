package http

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

type fakeExternalStore struct {
	user      store.User
	userErr   error
	server    store.Server
	serverErr error
}

func (f *fakeExternalStore) GetUserByExternalID(ctx context.Context, externalID string) (store.User, error) {
	return f.user, f.userErr
}

func (f *fakeExternalStore) GetServerByExternalID(ctx context.Context, externalID string) (store.Server, error) {
	return f.server, f.serverErr
}

func (f *fakeExternalStore) GetUserByID(ctx context.Context, id string) (store.User, error) {
	return f.user, f.userErr
}

func (f *fakeExternalStore) IsJWTRevoked(ctx context.Context, jti string) (bool, error) {
	return false, nil
}

func (f *fakeExternalStore) ValidateApiKey(ctx context.Context, key, ip string) (*store.User, []string, error) {
	return nil, nil, errors.New("invalid api key")
}

func TestExternalUserLookup_StoreNil(t *testing.T) {
	st := &fakeExternalStore{
		user: store.User{ID: "user-1", Email: "test@test.com", Role: "admin", SessionVersion: 1},
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	auth := authMiddlewareWithStore("secret", st, nil)
	grp := app.Group("/api/v1", auth)
	registerExternalLookupRoutes(grp, Config{Store: nil})

	req := httptest.NewRequest("GET", "/api/v1/users/external/ext-123", nil)
	req.Header.Set("Authorization", "Bearer "+sessionTokenForTest(t, "admin", "admin@test.com", 1))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 503 {
		t.Fatalf("expected 503 (no store), got %d", resp.StatusCode)
	}
}

func TestExternalServerLookup_StoreNil(t *testing.T) {
	st := &fakeExternalStore{
		user: store.User{ID: "user-1", Email: "admin@test.com", Role: "admin", SessionVersion: 1},
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	auth := authMiddlewareWithStore("secret", st, nil)
	grp := app.Group("/api/v1", auth)
	registerExternalLookupRoutes(grp, Config{Store: nil})

	req := httptest.NewRequest("GET", "/api/v1/servers/external/ext-456", nil)
	req.Header.Set("Authorization", "Bearer "+sessionTokenForTest(t, "admin", "admin@test.com", 1))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 503 {
		t.Fatalf("expected 503 (no store), got %d", resp.StatusCode)
	}
}
