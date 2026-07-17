package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOAuth2Provider_AuthCodeURL(t *testing.T) {
	provider := &OAuth2Provider{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		AuthURL:      "https://example.com/auth",
		TokenURL:     "https://example.com/token",
		RedirectURL:  "https://example.com/callback",
		Scopes:       []string{"scope1", "scope2"},
	}

	state := "test-state"
	authURL := provider.AuthCodeURL(state)

	expectedURL := "https://example.com/auth?client_id=test-client-id&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback&response_type=code&scope=scope1+scope2&state=test-state"
	if authURL != expectedURL {
		t.Errorf("AuthCodeURL() = %v, want %v", authURL, expectedURL)
	}
}

func TestOAuth2Provider_Exchange(t *testing.T) {
	// Mock OAuth2 server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		ClientSecret: "test-client-secret",
		AuthURL:      "https://example.com/auth",
		TokenURL:     ts.URL,
		RedirectURL:  "https://example.com/callback",
		Scopes:       []string{"scope1", "scope2"},
	}

	code := "test-code"
	token, err := provider.Exchange(context.Background(), code)
	if err != nil {
		t.Fatalf("Exchange() error = %v", err)
	}

	if token.AccessToken != "test-access-token" {
		t.Errorf("Exchange() AccessToken = %v, want %v", token.AccessToken, "test-access-token")
	}
}
