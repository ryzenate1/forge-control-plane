package phase1git

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"gamepanel/forge/internal/services/gitprovider"
	"gamepanel/forge/internal/store"
)

// defaultHTTPTimeout bounds all provider API calls made by the phase.
const defaultHTTPTimeout = 30 * time.Second

// oauthStateTTL is how long a connect state stays valid before the provider
// callback is rejected.
const oauthStateTTL = 15 * time.Minute

// BuildAuthorizeURL starts the git connect flow: it mints a single-use state
// row bound to the caller, then delegates to the existing
// gitprovider.OAuthAuthorizeURL. redirectTarget, when non-empty, is the
// relative path the browser is sent to after a successful callback.
func (b *Bridge) BuildAuthorizeURL(ctx context.Context, pt gitprovider.ProviderType, userID, redirectTarget string) (string, error) {
	if err := b.storageGuard(); err != nil {
		return "", err
	}
	cfg := b.providers.GetProviderConfig(pt)
	if cfg == nil || cfg.OAuth == nil {
		return "", fmt.Errorf("%w: %s", ErrNotConfigured, pt)
	}

	state, err := randomState()
	if err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}
	if err := b.storage.SetOAuthState(ctx, store.OAuthState{
		State:       state,
		UserID:      userID,
		Provider:    string(pt),
		RedirectURI: redirectTarget,
		ExpiresAt:   time.Now().UTC().Add(oauthStateTTL),
	}); err != nil {
		return "", fmt.Errorf("persist oauth state: %w", err)
	}

	authURL, err := b.providers.OAuthAuthorizeURL(pt, state)
	if err != nil {
		return "", fmt.Errorf("build authorize url: %w", err)
	}
	return authURL, nil
}

// CompleteOAuth exchanges a provider callback code for a token, resolves the
// connected user identity, and persists the token via
// gitprovider.ConnectProvider's contract onto git_provider_tokens. The
// single-use state is consumed on success or expiry.
func (b *Bridge) CompleteOAuth(ctx context.Context, pt gitprovider.ProviderType, userID, code, state string) (store.GitProviderToken, string, error) {
	if err := b.storageGuard(); err != nil {
		return store.GitProviderToken{}, "", err
	}
	st, err := b.storage.GetOAuthState(ctx, state)
	if err != nil {
		return store.GitProviderToken{}, "", fmt.Errorf("callback state: %w", err)
	}
	if st.UserID != userID {
		return store.GitProviderToken{}, "", fmt.Errorf("callback state does not belong to caller")
	}
	defer b.storage.DeleteOAuthState(ctx, state)

	token, err := b.providers.ExchangeOAuthCode(ctx, pt, code)
	if err != nil {
		return store.GitProviderToken{}, "", fmt.Errorf("exchange oauth code: %w", err)
	}

	cfg := b.providers.GetProviderConfig(pt)
	baseURL := ""
	if cfg != nil {
		baseURL = cfg.BaseURL
	}
	info, err := b.providers.GetUserInfo(ctx, pt, token.AccessToken, baseURL)
	if err != nil {
		return store.GitProviderToken{}, "", fmt.Errorf("fetch provider user: %w", err)
	}

	// ConnectProvider-equivalent persistence: the existing gitprovider method
	// writes through its own store, so we persist with the identical fields via
	// the shared store.CreateGitProviderToken contract the bridge uses.
	expiresAt := (*time.Time)(nil)
	if token.ExpiresIn > 0 {
		expiry := time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
		expiresAt = &expiry
	}
	persisted, err := b.storage.CreateGitProviderToken(ctx, store.CreateGitProviderTokenRequest{
		UserID:       userID,
		Provider:     store.GitProviderType(pt),
		ProviderName: string(pt) + "-" + info.Username,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		ExpiresAt:    expiresAt,
		Scope:        token.Scope,
		BaseURL:      baseURL,
		Username:     info.Username,
		AvatarURL:    info.AvatarURL,
	})
	if err != nil {
		return store.GitProviderToken{}, "", fmt.Errorf("persist provider token: %w", err)
	}
	return persisted, st.RedirectURI, nil
}

// ProviderConfigured reports whether a provider has OAuth credentials
// registered (used by the UI to decide between "connect" and "coming soon").
func (b *Bridge) ProviderConfigured(pt gitprovider.ProviderType) bool {
	cfg := b.providers.GetProviderConfig(pt)
	return cfg != nil && cfg.OAuth != nil && cfg.OAuth.ClientID != ""
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
