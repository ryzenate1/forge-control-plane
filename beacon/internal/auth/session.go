package auth

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

var ErrSessionNotFound = errors.New("session not found")
var ErrSessionExpired = errors.New("session expired")

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
	mu         sync.RWMutex
	sessions   map[string]Session
}

func NewCookieSessionStore(cookieName string, secure, httpOnly bool, sameSite http.SameSite) *CookieSessionStore {
	return &CookieSessionStore{
		cookieName: cookieName,
		secure:     secure,
		httpOnly:   httpOnly,
		sameSite:   sameSite,
		sessions:   make(map[string]Session),
	}
}

func (s *CookieSessionStore) Create(ctx context.Context, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

func (s *CookieSessionStore) Get(ctx context.Context, id string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		delete(s.sessions, id)
		return Session{}, ErrSessionExpired
	}
	return session, nil
}

func (s *CookieSessionStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return ErrSessionNotFound
	}
	delete(s.sessions, id)
	return nil
}
