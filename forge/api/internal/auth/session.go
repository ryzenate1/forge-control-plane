package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

type Session struct {
	ID           string    `json:"id"`
	UserID       string    `json:"userId"`
	Token        string    `json:"token"`
	CreatedAt    time.Time `json:"createdAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
	LastActiveAt time.Time `json:"lastActiveAt"`
	IPAddress    string    `json:"ipAddress"`
	UserAgent    string    `json:"userAgent"`
}

type SessionStore interface {
	Create(ctx context.Context, session *Session) error
	Get(ctx context.Context, id string) (*Session, error)
	GetByToken(ctx context.Context, token string) (*Session, error)
	Update(ctx context.Context, session *Session) error
	Delete(ctx context.Context, id string) error
	DeleteByUser(ctx context.Context, userID string) error
	ListByUser(ctx context.Context, userID string) ([]*Session, error)
	Cleanup(ctx context.Context) error
}

type InMemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	byToken  map[string]string
	byUser   map[string][]string
}

func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[string]*Session),
		byToken:  make(map[string]string),
		byUser:   make(map[string][]string),
	}
}

func (s *InMemorySessionStore) Create(_ context.Context, session *Session) error {
	if session == nil {
		return errors.New("session: nil session")
	}
	if session.ID == "" || session.Token == "" || session.UserID == "" {
		return errors.New("session: id, token, and userId are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.ID] = session
	s.byToken[session.Token] = session.ID
	s.byUser[session.UserID] = append(s.byUser[session.UserID], session.ID)
	return nil
}

func (s *InMemorySessionStore) Get(_ context.Context, id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessions[id]
	if !ok {
		return nil, errors.New("session: not found")
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, errors.New("session: expired")
	}
	return sess, nil
}

func (s *InMemorySessionStore) GetByToken(_ context.Context, token string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.byToken[token]
	if !ok {
		return nil, errors.New("session: not found")
	}
	sess, ok := s.sessions[id]
	if !ok {
		return nil, errors.New("session: not found")
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, errors.New("session: expired")
	}
	return sess, nil
}

func (s *InMemorySessionStore) Update(_ context.Context, session *Session) error {
	if session == nil {
		return errors.New("session: nil session")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[session.ID]; !ok {
		return errors.New("session: not found")
	}
	s.sessions[session.ID] = session
	return nil
}

func (s *InMemorySessionStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if !ok {
		return nil
	}

	delete(s.byToken, sess.Token)
	delete(s.sessions, id)

	if userSessions, exists := s.byUser[sess.UserID]; exists {
		for i, sid := range userSessions {
			if sid == id {
				s.byUser[sess.UserID] = append(userSessions[:i], userSessions[i+1:]...)
				break
			}
		}
		if len(s.byUser[sess.UserID]) == 0 {
			delete(s.byUser, sess.UserID)
		}
	}

	return nil
}

func (s *InMemorySessionStore) DeleteByUser(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := s.byUser[userID]
	for _, id := range ids {
		if sess, ok := s.sessions[id]; ok {
			delete(s.byToken, sess.Token)
			delete(s.sessions, id)
		}
	}
	delete(s.byUser, userID)
	return nil
}

func (s *InMemorySessionStore) Cleanup(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.byToken, sess.Token)
			delete(s.sessions, id)
			if userSessions, exists := s.byUser[sess.UserID]; exists {
				for i, sid := range userSessions {
					if sid == id {
						s.byUser[sess.UserID] = append(userSessions[:i], userSessions[i+1:]...)
						break
					}
				}
				if len(s.byUser[sess.UserID]) == 0 {
					delete(s.byUser, sess.UserID)
				}
			}
		}
	}
	return nil
}

func (s *InMemorySessionStore) ListByUser(_ context.Context, userID string) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.byUser[userID]
	result := make([]*Session, 0, len(ids))
	for _, id := range ids {
		if sess, ok := s.sessions[id]; ok {
			result = append(result, sess)
		}
	}
	return result, nil
}

func GenerateSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.New("session: failed to generate token: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func GenerateSessionID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.New("session: failed to generate id: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

type sessionContextKey struct{}

func SessionFromContext(ctx context.Context) *Session {
	if sess, ok := ctx.Value(sessionContextKey{}).(*Session); ok {
		return sess
	}
	return nil
}

func ContextWithSession(ctx context.Context, sess *Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, sess)
}

const sessionFiberLocalsKey = "auth_session"

func SessionMiddleware(store SessionStore) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Cookies("__Host-forge_session_v2")
		if token == "" {
			authHeader := c.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				token = authHeader[7:]
			}
		}

		if token == "" {
			return c.Next()
		}

		sess, err := store.GetByToken(c.Context(), token)
		if err != nil {
			return c.Next()
		}

		if time.Now().After(sess.ExpiresAt) {
			return c.Next()
		}

		sess.LastActiveAt = time.Now()
		_ = store.Update(c.Context(), sess)

		c.Locals(sessionFiberLocalsKey, sess)
		return c.Next()
	}
}

func GetSessionFromFiberCtx(c *fiber.Ctx) *Session {
	if sess, ok := c.Locals(sessionFiberLocalsKey).(*Session); ok {
		return sess
	}
	return nil
}

const SessionCookieName = "__Host-forge_session_v2"

func SetSessionCookie(c *fiber.Ctx, token string, expiresAt time.Time) {
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Expires:  expiresAt,
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
		Path:     "/",
	})
}

func ClearSessionCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Expires:  time.Unix(0, 0),
		HTTPOnly: true,
		Secure:   true,
		SameSite: "Lax",
		Path:     "/",
	})
}

func ValidateSessionIP(sess *Session, currentIP string) bool {
	return sess.IPAddress == "" || sess.IPAddress == currentIP
}
