package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

type oauthRequest struct {
	verifier string
	expires  time.Time
}

type OAuth2Provider struct {
	ClientID     string
	ClientSecret []byte
	AuthURL      string
	TokenURL     string
	RedirectURL  string
	Scopes       []string

	mu      sync.Mutex
	pending map[string]oauthRequest
}

func (p *OAuth2Provider) config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: string(p.ClientSecret),
		RedirectURL:  p.RedirectURL,
		Scopes:       append([]string(nil), p.Scopes...),
		Endpoint: oauth2.Endpoint{
			AuthURL: p.AuthURL, TokenURL: p.TokenURL,
		},
	}
}

// AuthCodeURL creates server-owned state and a PKCE S256 challenge. The state
// must be passed unchanged to Exchange.
func (p *OAuth2Provider) AuthCodeURL() (authURL, state string, err error) {
	state, err = randomOAuthValue(32)
	if err != nil {
		return "", "", err
	}
	verifier, err := randomOAuthValue(64)
	if err != nil {
		return "", "", err
	}
	challengeSum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeSum[:])

	p.mu.Lock()
	if p.pending == nil {
		p.pending = make(map[string]oauthRequest)
	}
	now := time.Now()
	for key, request := range p.pending {
		if !request.expires.After(now) {
			delete(p.pending, key)
		}
	}
	if len(p.pending) >= 1024 {
		p.mu.Unlock()
		return "", "", errors.New("too many pending OAuth requests")
	}
	p.pending[state] = oauthRequest{verifier: verifier, expires: now.Add(10 * time.Minute)}
	p.mu.Unlock()

	authURL = p.config().AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	return authURL, state, nil
}

func (p *OAuth2Provider) Exchange(ctx context.Context, code, state string) (*oauth2.Token, error) {
	p.mu.Lock()
	request, exists := p.pending[state]
	delete(p.pending, state)
	p.mu.Unlock()
	if !exists || !request.expires.After(time.Now()) {
		return nil, errors.New("invalid or expired OAuth state")
	}
	return p.config().Exchange(ctx, code, oauth2.VerifierOption(request.verifier))
}

func (p *OAuth2Provider) ClearSecret() {
	for index := range p.ClientSecret {
		p.ClientSecret[index] = 0
	}
	p.ClientSecret = nil
}

func randomOAuthValue(size int) (string, error) {
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}
