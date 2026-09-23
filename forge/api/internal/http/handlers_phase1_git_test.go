package http

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"gamepanel/forge/internal/services/gitprovider"
	"gamepanel/forge/internal/services/phase1git"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// fakePhase1Storage is an in-memory double for phase1git.Storage.
type fakePhase1Storage struct {
	states map[string]store.OAuthState
	tokens []store.GitProviderToken
}

func newFakePhase1Storage() *fakePhase1Storage {
	return &fakePhase1Storage{states: make(map[string]store.OAuthState)}
}

func (f *fakePhase1Storage) SetOAuthState(_ context.Context, st store.OAuthState) error {
	f.states[st.State] = st
	return nil
}
func (f *fakePhase1Storage) GetOAuthState(_ context.Context, state string) (store.OAuthState, error) {
	st, ok := f.states[state]
	if !ok {
		return store.OAuthState{}, &storeNotFoundError{"oauth state not found"}
	}
	return st, nil
}
func (f *fakePhase1Storage) DeleteOAuthState(_ context.Context, state string) error {
	delete(f.states, state)
	return nil
}
func (f *fakePhase1Storage) CreateGitProviderToken(_ context.Context, req store.CreateGitProviderTokenRequest) (store.GitProviderToken, error) {
	tok := store.GitProviderToken{
		ID:           "tok-" + req.Username,
		UserID:       req.UserID,
		Provider:     req.Provider,
		ProviderName: req.ProviderName,
		AccessToken:  req.AccessToken,
		Username:     req.Username,
		BaseURL:      req.BaseURL,
	}
	f.tokens = append(f.tokens, tok)
	return tok, nil
}
func (f *fakePhase1Storage) CreateGitWebhookEvent(_ context.Context, e store.GitWebhookEvent) error {
	return nil
}

type storeNotFoundError struct{ msg string }

func (e *storeNotFoundError) Error() string { return e.msg }

func testPhase1Bridge(t *testing.T) (*phase1git.Bridge, *fakePhase1Storage) {
	t.Helper()
	svc := gitprovider.NewService(nil, nil)
	// Register GitHub OAuth so authorize/callback are considered configured.
	svc.RegisterProviderConfig(&gitprovider.ProviderConfig{
		Type: gitprovider.ProviderGitHub,
		Name: "GitHub",
		OAuth: &gitprovider.OAuthConfig{
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
			RedirectURL:  "https://panel.example.com/api/v1/git/oauth/github/callback",
		},
	})
	st := newFakePhase1Storage()
	bridge := phase1git.New(svc, nil, nil)
	bridge.WithStorage(st)
	return bridge, st
}

func TestPhase1GitOAuthAuthorize(t *testing.T) {
	bridge, st := testPhase1Bridge(t)
	app := fiber.New()
	app.Get("/git/oauth/:provider/authorize", func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Sub: "user-1", Role: "user"})
		return Phase1GitOAuthAuthorize(bridge)(c)
	})

	t.Run("configured provider redirects", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/git/oauth/github/authorize?redirect_to=/console/git", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 302 {
			t.Fatalf("expected 302, got %d", resp.StatusCode)
		}
		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "github.com/login/oauth/authorize") {
			t.Errorf("redirect should go to github, got %q", loc)
		}
		if !strings.Contains(loc, "client_id=test-client-id") {
			t.Errorf("redirect should contain client_id, got %q", loc)
		}
		// State should have been persisted.
		if len(st.states) != 1 {
			t.Fatalf("expected 1 stored state, got %d", len(st.states))
		}
		for _, s := range st.states {
			if s.UserID != "user-1" {
				t.Errorf("state user = %q, want user-1", s.UserID)
			}
			if s.RedirectURI != "/console/git" {
				t.Errorf("redirectURI = %q, want /console/git", s.RedirectURI)
			}
		}
	})

	t.Run("unconfigured provider returns 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/git/oauth/gitea/authorize", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 400 {
			t.Errorf("expected 400 for unconfigured provider, got %d", resp.StatusCode)
		}
	})

	t.Run("unsupported provider returns 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/git/oauth/frobnicate/authorize", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		// Unsupported provider is validation (invalid provider) → 422 per normalized status codes.
		if resp.StatusCode != 422 {
			t.Errorf("expected 422 for unsupported provider, got %d", resp.StatusCode)
		}
	})

	t.Run("unauthenticated returns 401", func(t *testing.T) {
		app2 := fiber.New()
		app2.Get("/git/oauth/:provider/authorize", Phase1GitOAuthAuthorize(bridge))
		req := httptest.NewRequest("GET", "/git/oauth/github/authorize", nil)
		resp, _ := app2.Test(req)
		if resp.StatusCode != 401 {
			t.Errorf("expected 401 without auth, got %d", resp.StatusCode)
		}
	})
}

func TestPhase1GitOAuthCallback(t *testing.T) {
	bridge, st := testPhase1Bridge(t)
	cfg := Config{Store: nil}
	app := fiber.New()
	app.Get("/git/oauth/:provider/callback", func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Sub: "user-1", Role: "user"})
		return Phase1GitOAuthCallback(cfg, bridge)(c)
	})

	// Pre-seed a state for user-1.
	st.states["state-123"] = store.OAuthState{
		State:       "state-123",
		UserID:      "user-1",
		Provider:    "github",
		RedirectURI: "/console/git",
	}

	t.Run("missing code or state returns 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/git/oauth/github/callback?code=&state=state-123", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 400 {
			t.Errorf("expected 400 for missing code, got %d", resp.StatusCode)
		}
		req2 := httptest.NewRequest("GET", "/git/oauth/github/callback?code=abc&state=", nil)
		resp2, _ := app.Test(req2)
		if resp2.StatusCode != 400 {
			t.Errorf("expected 400 for missing state, got %d", resp2.StatusCode)
		}
	})

	t.Run("state belonging to different user fails", func(t *testing.T) {
		// state-123 belongs to user-1, but request is from user-2.
		app2 := fiber.New()
		app2.Get("/git/oauth/:provider/callback", func(c *fiber.Ctx) error {
			c.Locals("user", tokenClaims{Sub: "user-2", Role: "user"})
			return Phase1GitOAuthCallback(cfg, bridge)(c)
		})
		req := httptest.NewRequest("GET", "/git/oauth/github/callback?code=abc&state=state-123", nil)
		resp, _ := app2.Test(req)
		if resp.StatusCode != 400 {
			t.Errorf("expected 400 for state user mismatch, got %d", resp.StatusCode)
		}
	})

	t.Run("unknown state returns 400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/git/oauth/github/callback?code=abc&state=unknown-state", nil)
		resp, _ := app.Test(req)
		if resp.StatusCode != 400 {
			t.Errorf("expected 400 for unknown state, got %d", resp.StatusCode)
		}
	})

	// Note: full success path (exchange + userinfo) is tested via
	// Bridge unit tests with mocked HTTP transport; here we verify the
	// handler correctly validates state ownership and delegates to the
	// bridge before attempting the network call.
}

// TestGitProviderHandlesAllTokens verifies gitprovider's OAuth registry handles
// GitHub, GitLab, Bitbucket, and Gitea provider tokens correctly via the
// canonical providerFactory dispatch (OAuthAuthorizeURL). Each provider is
// registered with its own OAuth config and the authorize URL is verified to
// contain the provider-specific host.
func TestGitProviderHandlesAllTokens(t *testing.T) {
	svc := gitprovider.NewService(nil, nil)
	providers := []struct {
		pt   gitprovider.ProviderType
		base string
		host string
	}{
		{gitprovider.ProviderGitHub, "", "github.com"},
		{gitprovider.ProviderGitLab, "", "gitlab.com"},
		{gitprovider.ProviderBitbucket, "", "bitbucket.org"},
		{gitprovider.ProviderGitea, "https://gitea.example.com", "gitea.example.com"},
	}
	for _, p := range providers {
		cfg := &gitprovider.ProviderConfig{
			Type:    p.pt,
			Name:    string(p.pt),
			BaseURL: p.base,
			OAuth: &gitprovider.OAuthConfig{
				ClientID:     "id-" + string(p.pt),
				ClientSecret: "secret-" + string(p.pt),
				RedirectURL:  "https://panel.example.com/callback",
			},
		}
		svc.RegisterProviderConfig(cfg)
		url, err := svc.OAuthAuthorizeURL(p.pt, "state-xyz")
		if err != nil {
			t.Fatalf("provider %s: unexpected error: %v", p.pt, err)
		}
		if !strings.Contains(url, p.host) {
			t.Errorf("provider %s: url %q should contain host %q", p.pt, url, p.host)
		}
		if !strings.Contains(url, "state-xyz") {
			t.Errorf("provider %s: url %q should contain state", p.pt, url)
		}
		// Verify GetProviderConfig returns the registered config.
		got := svc.GetProviderConfig(p.pt)
		if got == nil || got.OAuth.ClientID != "id-"+string(p.pt) {
			t.Errorf("provider %s: GetProviderConfig mismatch", p.pt)
		}
	}
	// Unregistered provider should error.
	if _, err := svc.OAuthAuthorizeURL(gitprovider.ProviderType("unknown"), "s"); err == nil {
		t.Error("expected error for unknown provider")
	}
}
