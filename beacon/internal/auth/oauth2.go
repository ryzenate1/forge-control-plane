package auth

import (
	"context"

	"golang.org/x/oauth2"
)

type OAuth2Provider struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	RedirectURL  string
	Scopes       []string
}

func (p *OAuth2Provider) AuthCodeURL(state string) string {
	config := &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		RedirectURL:  p.RedirectURL,
		Scopes:       p.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  p.AuthURL,
			TokenURL: p.TokenURL,
		},
	}
	return config.AuthCodeURL(state)
}

func (p *OAuth2Provider) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	config := &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		RedirectURL:  p.RedirectURL,
		Scopes:       p.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  p.AuthURL,
			TokenURL: p.TokenURL,
		},
	}
	return config.Exchange(ctx, code)
}
