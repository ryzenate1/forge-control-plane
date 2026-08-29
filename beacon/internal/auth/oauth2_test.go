package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestOAuth2Provider_AuthCodeURL(t *testing.T) {
	provider := &OAuth2Provider{
		ClientID:     "test-client-id",
		ClientSecret: []byte("test-client-secret"),
		AuthURL:      "https://example.com/auth",
		TokenURL:     "https://example.com/token",
		RedirectURL:  "https://example.com/callback",
		Scopes:       []string{"scope1", "scope2"},
	}

	authURL, state, err := provider.AuthCodeURL()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	if state == "" || parsed.Query().Get("state") != state {
		t.Fatal("generated OAuth state is missing")
	}
	if parsed.Query().Get("code_challenge_method") != "S256" || parsed.Query().Get("code_challenge") == "" {
		t.Fatal("PKCE S256 challenge is missing")
	}
}

func TestOAuth2Provider_Exchange(t *testing.T) {
	// Mock OAuth2 server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("code_verifier") == "" {
			http.Error(w, "missing verifier", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
            "access_token": "test-access-token",
            "token_type": "Bearer",
            "expires_in": 3600
        }`))
	}))
	defer ts.Close()

	provider := &OAuth2Provider{
		ClientID:     "test-client-id",
		ClientSecret: []byte("test-client-secret"),
		AuthURL:      "https://example.com/auth",
		TokenURL:     ts.URL,
		RedirectURL:  "https://example.com/callback",
		Scopes:       []string{"scope1", "scope2"},
	}

	code := "test-code"
	_, state, err := provider.AuthCodeURL()
	if err != nil {
		t.Fatal(err)
	}
	token, err := provider.Exchange(context.Background(), code, state)
	if err != nil {
		t.Fatalf("Exchange() error = %v", err)
	}

	if token.AccessToken != "test-access-token" {
		t.Errorf("Exchange() AccessToken = %v, want %v", token.AccessToken, "test-access-token")
	}
	if _, err := provider.Exchange(context.Background(), code, state); err == nil {
		t.Fatal("OAuth state replay was accepted")
	}
}
