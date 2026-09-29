package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
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

// CheckConfig refuses a provider that cannot complete a safe ceremony: a
// missing client id, a missing or non-absolute redirect URI, or a cleartext
// endpoint. A prefix-matched or wildcard redirect is what turns an OAuth
// callback into an account takeover, so the redirect must be the exact
// absolute URL registered with the identity provider.
func (p *OAuth2Provider) CheckConfig() error {
	if p == nil {
		return errors.New("oauth2 provider not configured")
	}
	if strings.TrimSpace(p.ClientID) == "" {
		return errors.New("oauth2: client id is required")
	}
	if len(p.ClientSecret) == 0 {
		return errors.New("oauth2: client secret is required")
	}
	if err := checkEndpoint("authorization", p.AuthURL); err != nil {
		return err
	}
	if err := checkEndpoint("token", p.TokenURL); err != nil {
		return err
	}
	redirect := strings.TrimSpace(p.RedirectURL)
	parsed, err := url.Parse(redirect)
	if err != nil {
		return fmt.Errorf("oauth2: redirect uri is not a valid URL: %w", err)
	}
	if !parsed.IsAbs() || parsed.Host == "" || parsed.Fragment != "" {
		return errors.New("oauth2: redirect uri must be an absolute URL without a fragment")
	}
	return checkEndpoint("redirect", redirect)
}

// checkEndpoint allows cleartext only for a loopback host, which is a local
// identity provider in development or a test server. Anything else over http
// ships the authorization code, the PKCE verifier and the client secret to
// whoever can read the path.
func checkEndpoint(label, raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("oauth2: %s endpoint is not a valid URL: %w", label, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("oauth2: %s endpoint must be an absolute URL", label)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return nil
	case "http":
		host := parsed.Hostname()
		if strings.EqualFold(host, "localhost") {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("oauth2: %s endpoint uses cleartext http: %s", label, parsed.Redacted())
	default:
		return fmt.Errorf("oauth2: %s endpoint has an unsupported scheme %q", label, parsed.Scheme)
	}
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
	if err := p.CheckConfig(); err != nil {
		return "", "", err
	}
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
	if err := p.CheckConfig(); err != nil {
		return nil, err
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("missing authorization code")
	}
	// The state is removed before anything is checked: a state that reaches
	// this function has one use, whether it then validates or not. A replay
	// of a captured callback therefore cannot re-drive an in-flight ceremony.
	p.mu.Lock()
	request, exists := p.pending[state]
	delete(p.pending, state)
	p.mu.Unlock()
	if !exists || !request.expires.After(time.Now()) {
		return nil, errors.New("invalid or expired OAuth state")
	}
	token, err := p.config().Exchange(ctx, code, oauth2.VerifierOption(request.verifier))
	if err != nil {
		// The upstream error can echo the request, which carried the code and
		// the verifier; report only that the exchange failed.
		return nil, errors.New("oauth2 token exchange failed")
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, errors.New("oauth2 provider returned no access token")
	}
	return token, nil
}

// VerifyRedirect reports whether a callback URI is exactly the registered one.
// Comparison is whole-string on the normalised URL: a prefix match would let
// https://rp.example/callback-attacker through.
func (p *OAuth2Provider) VerifyRedirect(candidate string) bool {
	if p == nil {
		return false
	}
	want, err := url.Parse(strings.TrimSpace(p.RedirectURL))
	got, gotErr := url.Parse(strings.TrimSpace(candidate))
	if err != nil || gotErr != nil {
		return false
	}
	return strings.EqualFold(want.Scheme, got.Scheme) &&
		strings.EqualFold(want.Host, got.Host) &&
		strings.EqualFold(want.EscapedPath(), got.EscapedPath())
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
