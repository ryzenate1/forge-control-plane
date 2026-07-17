package auth

import (
	"context"
	"net/http"
	"time"
)

type Session struct {
	ID        string
	UserID    string
	Scopes    Scopes
	CreatedAt time.Time
	ExpiresAt time.Time
}

type SessionStore interface {
	Create(ctx context.Context, s Session) error
	Get(ctx context.Context, id string) (Session, error)
	Delete(ctx context.Context, id string) error
}

type CookieSessionStore struct {
	cookieName string
	secure     bool
	httpOnly   bool
	sameSite   http.SameSite
}

func NewCookieSessionStore(cookieName string, secure, httpOnly bool, sameSite http.SameSite) *CookieSessionStore {
	return &CookieSessionStore{
		cookieName: cookieName,
		secure:     secure,
		httpOnly:   httpOnly,
		sameSite:   sameSite,
	}
}

func (s *CookieSessionStore) Create(ctx context.Context, session Session) error {
	// Implementation would set the session cookie
	return nil
}

func (s *CookieSessionStore) Get(ctx context.Context, id string) (Session, error) {
	// Implementation would retrieve the session from the cookie
	return Session{}, nil
}

func (s *CookieSessionStore) Delete(ctx context.Context, id string) error {
	// Implementation would delete the session cookie
	return nil
}
