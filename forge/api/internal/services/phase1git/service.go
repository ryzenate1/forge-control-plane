// Package phase1git provides the "Domain + Git substrate" phase's git-facing
// API surface: Git OAuth connect (a thin, route-bound sled over the existing
// gitprovider service), provider repo browsing (tree/contents/readme/commits),
// auto-provisioning of deploy keys, and the webhook event record helpers.
package phase1git

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"gamepanel/forge/internal/services/gitprovider"
	"gamepanel/forge/internal/store"
)

// ErrNotConfigured is returned when a provider has no OAuth config registered.
var ErrNotConfigured = errors.New("oauth not configured for provider")

// Storage is the persistence slice the phase needs. *store.Store satisfies it
// directly; tests supply in-memory doubles against the same contract.
type Storage interface {
	SetOAuthState(ctx context.Context, st store.OAuthState) error
	GetOAuthState(ctx context.Context, state string) (store.OAuthState, error)
	DeleteOAuthState(ctx context.Context, state string) error
	CreateGitProviderToken(ctx context.Context, req store.CreateGitProviderTokenRequest) (store.GitProviderToken, error)
	CreateGitWebhookEvent(ctx context.Context, e store.GitWebhookEvent) error
}

// Bridge is the phase's service face. It reuses gitprovider's OAuth
// primitives and the store's git_provider_tokens persistence, adding the
// state table enforcement and provider API helpers (browse, deploy keys).
type Bridge struct {
	providers *gitprovider.Service
	storage   Storage
	httpC     *http.Client
	logger    *slog.Logger
}

// New builds a Bridge against the concrete store and gitprovider service.
func New(providers *gitprovider.Service, stg *store.Store, logger *slog.Logger) *Bridge {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bridge{
		providers: providers,
		storage:   stg,
		httpC:     &http.Client{Timeout: defaultHTTPTimeout},
		logger:    logger,
	}
}

// WithStorage overrides the persistence layer (used by tests).
func (b *Bridge) WithStorage(stg Storage) *Bridge {
	if stg != nil {
		b.storage = stg
	}
	return b
}

func (b *Bridge) Providers() *gitprovider.Service {
	return b.providers
}

func (b *Bridge) ErrNotConfigured() error {
	return ErrNotConfigured
}

// storageGuard returns an error when the bridge has no persistence wiring.
func (b *Bridge) storageGuard() error {
	if b.storage == nil {
		return errors.New("storage not available")
	}
	return nil
}
