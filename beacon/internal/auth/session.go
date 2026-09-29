package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

var ErrSessionNotFound = errors.New("session not found")
var ErrSessionExpired = errors.New("session expired")
var ErrSessionStoreFull = errors.New("session store is at capacity")

// maxSessions bounds the in-memory session map. Reaching it denies new logins
// rather than evicting somebody's live session: an evicted session is a
// silent revocation, and an unbounded map on a root process is an outage.
const maxSessions = 100_000

// Session is a server-side login record. Expiry is mandatory: a session with
// no end date is a permanent credential held in a process that can be
// restarted, and it cannot be rotated out.
type Session struct {
	ID        string
	UserID    string
	Scopes    Scopes
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (s Session) expired(now time.Time) bool {
	return s.ExpiresAt.IsZero() || !now.Before(s.ExpiresAt)
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
	if strings.TrimSpace(cookieName) == "" {
		cookieName = "session"
	}
	return &CookieSessionStore{
		cookieName: cookieName,
		secure:     secure,
		httpOnly:   httpOnly,
		sameSite:   sameSite,
		sessions:   make(map[string]Session),
	}
}

func (s *CookieSessionStore) Create(ctx context.Context, session Session) error {
	if s == nil {
		return errors.New("session store not initialised")
	}
	session.ID = strings.TrimSpace(session.ID)
	session.UserID = strings.TrimSpace(session.UserID)
	if session.ID == "" || session.UserID == "" {
		return errors.New("session requires an id and a principal")
	}
	// A zero expiry used to be read as "never expires". Refuse it: the store
	// cannot promise to retire a credential it does not know the end of.
	if session.ExpiresAt.IsZero() || !time.Now().Before(session.ExpiresAt) {
		return ErrSessionExpired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = make(map[string]Session)
	}
	if _, exists := s.sessions[session.ID]; !exists && len(s.sessions) >= maxSessions {
		s.sweepLocked(time.Now())
		if len(s.sessions) >= maxSessions {
			return ErrSessionStoreFull
		}
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	s.sessions[session.ID] = session
	return nil
}

func (s *CookieSessionStore) Get(ctx context.Context, id string) (Session, error) {
	if s == nil {
		return Session{}, ErrSessionNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, ErrSessionNotFound
	}
	// Taking the write lock, not a read lock: an expired session is deleted
	// here, and mutating a map under RLock is a concurrent map write - a
	// fatal, unrecoverable runtime error in this process.
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	now := time.Now()
	if session.expired(now) {
		delete(s.sessions, id)
		return Session{}, ErrSessionExpired
	}
	return session, nil
}

func (s *CookieSessionStore) Delete(ctx context.Context, id string) error {
	if s == nil {
		return errors.New("session store not initialised")
	}
	id = strings.TrimSpace(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return ErrSessionNotFound
	}
	delete(s.sessions, id)
	return nil
}

// Rotate replaces a session with a fresh identity while carrying the owner and
// scopes forward, and retires the old record. Logging in without rotating is
// session fixation: whatever id the browser already held becomes authenticated.
func (s *CookieSessionStore) Rotate(ctx context.Context, oldID string, next Session) (Session, error) {
	if s == nil {
		return Session{}, errors.New("session store not initialised")
	}
	previous, err := s.Get(ctx, oldID)
	if err != nil {
		return Session{}, err
	}
	if next.UserID == "" {
		next.UserID = previous.UserID
	}
	if next.Scopes == nil {
		next.Scopes = previous.Scopes
	}
	if next.ID == "" {
		issued, err := NewSessionID()
		if err != nil {
			return Session{}, err
		}
		next.ID = issued
	}
	if err := s.Create(ctx, next); err != nil {
		return Session{}, err
	}
	// The old id is retired only after the new one exists, so a failure here
	// cannot leave the principal with no session at all.
	_ = s.Delete(ctx, previous.ID)
	return next, nil
}

// NewSessionID mints a 256-bit session identifier. A predictable id is the same
// as no authentication at all.
func NewSessionID() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// SetCookie writes the session cookie using the attributes the store was built
// with. Handlers must not build the cookie themselves: an ad-hoc http.Cookie
// defaults to no HttpOnly and no Secure, which discards the protection this
// store was configured to apply.
func (s *CookieSessionStore) SetCookie(w http.ResponseWriter, id string, expires time.Time) {
	if s == nil || w == nil {
		return
	}
	cookie := &http.Cookie{
		Name:     s.cookieName,
		Value:    strings.TrimSpace(id),
		Path:     "/",
		Expires:  expires,
		Secure:   s.secure,
		HttpOnly: s.httpOnly,
		SameSite: s.sameSite,
	}
	if cookie.SameSite == http.SameSiteNoneMode {
		// Browsers reject SameSite=None without Secure, and a cookie the
		// browser drops is a login that silently never persists.
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
}

// ClearCookie expires the session cookie on the client.
func (s *CookieSessionStore) ClearCookie(w http.ResponseWriter) {
	if s == nil || w == nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		Secure:   s.secure,
		HttpOnly: s.httpOnly,
		SameSite: s.sameSite,
	})
}

// Authenticate resolves the session carried by the request's cookie. A missing,
// blank or unknown cookie is ErrSessionNotFound: there is no anonymous session.
func (s *CookieSessionStore) Authenticate(r *http.Request) (Session, error) {
	if s == nil || r == nil {
		return Session{}, ErrSessionNotFound
	}
	cookie, err := r.Cookie(s.cookieName)
	if err != nil {
		return Session{}, ErrSessionNotFound
	}
	if cookie == nil || strings.TrimSpace(cookie.Value) == "" {
		return Session{}, ErrSessionNotFound
	}
	return s.Get(r.Context(), cookie.Value)
}

// Touch extends a live session's expiry, keeping the write under the same lock
// so a concurrent refresh cannot leave a torn record behind.
func (s *CookieSessionStore) Touch(ctx context.Context, id string, expiresAt time.Time) error {
	if s == nil {
		return errors.New("session store not initialised")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[strings.TrimSpace(id)]
	if !ok {
		return ErrSessionNotFound
	}
	if session.expired(time.Now()) {
		delete(s.sessions, strings.TrimSpace(id))
		return ErrSessionExpired
	}
	if expiresAt.IsZero() || !time.Now().Before(expiresAt) {
		return ErrSessionExpired
	}
	session.ExpiresAt = expiresAt
	s.sessions[session.ID] = session
	return nil
}

// Cleanup drops expired sessions so the map cannot accumulate records that
// nothing can reach any more.
func (s *CookieSessionStore) Cleanup() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(time.Now())
}

func (s *CookieSessionStore) sweepLocked(now time.Time) {
	for id, session := range s.sessions {
		if session.expired(now) {
			delete(s.sessions, id)
		}
	}
}

// Len reports how many sessions are held, for capacity alerting.
func (s *CookieSessionStore) Len() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}
