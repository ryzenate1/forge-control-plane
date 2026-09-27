package webauthn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

// registrationSessionTTL / loginSessionTTL bound how long an issued challenge
// stays redeemable.
const (
	registrationSessionTTL = 5 * time.Minute
	loginSessionTTL        = 5 * time.Minute
)

type WebAuthnCredential struct {
	ID              string    `json:"id"`
	UserID          string    `json:"userId"`
	CredentialID    []byte    `json:"credentialId"`
	PublicKey       []byte    `json:"publicKey"`
	AttestationType string    `json:"attestationType"`
	AAGUID          []byte    `json:"aaguid"`
	SignCount       uint32    `json:"signCount"`
	CloneWarning    bool      `json:"cloneWarning"`
	Name            string    `json:"name"`
	CreatedAt       time.Time `json:"createdAt"`
	LastUsedAt      time.Time `json:"lastUsedAt"`
}

type WebAuthnUser struct {
	ID          string
	Name        string
	DisplayName string
	Credentials []webauthn.Credential
}

func (u *WebAuthnUser) WebAuthnID() []byte { return []byte(u.ID) }

func (u *WebAuthnUser) WebAuthnName() string { return u.Name }

func (u *WebAuthnUser) WebAuthnDisplayName() string { return u.DisplayName }

func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

type CredentialStore interface {
	GetCredentials(ctx context.Context, userID string) ([]WebAuthnCredential, error)
	SaveCredential(ctx context.Context, userID string, cred WebAuthnCredential) error
	RemoveCredential(ctx context.Context, userID, credentialID string) error
	// RecordCredentialUsage persists the signer counter and clone verdict after
	// a successful assertion. Without it every login is checked against the
	// count captured at registration, so authenticator-clone detection can
	// never fire.
	RecordCredentialUsage(ctx context.Context, userID, credentialRowID string, signCount uint32, cloneWarning bool, lastUsedAt time.Time) error
}

type SessionStore interface {
	Save(ctx context.Context, key string, data []byte, expiry time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

// SingleUseSessionStore is an optional SessionStore extension: implementations
// that can atomically read-and-remove a session. When it is absent the service
// falls back to Get+Delete, which still consumes the challenge before it is
// verified but leaves a small concurrency window.
type SingleUseSessionStore interface {
	GetDelete(ctx context.Context, key string) ([]byte, error)
}

type Service struct {
	wa           *webauthn.WebAuthn
	credStore    CredentialStore
	sessionStore SessionStore
}

func New(rpID, rpDisplayName, rpOrigin string, credStore CredentialStore, sessionStore SessionStore) (*Service, error) {
	origins := make([]string, 0, 2)
	for _, origin := range strings.Split(rpOrigin, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			origins = append(origins, origin)
		}
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: rpDisplayName,
		RPID:          rpID,
		RPOrigins:     origins,
	})
	if err != nil {
		return nil, err
	}
	return &Service{wa: wa, credStore: credStore, sessionStore: sessionStore}, nil
}

// consumeSession reads a ceremony session and removes it in one step so a
// challenge can never be replayed, not even against a failed verification.
func (s *Service) consumeSession(ctx context.Context, key string) ([]byte, error) {
	if atomic, ok := s.sessionStore.(SingleUseSessionStore); ok {
		return atomic.GetDelete(ctx, key)
	}
	data, err := s.sessionStore.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if delErr := s.sessionStore.Delete(ctx, key); delErr != nil {
		return nil, fmt.Errorf("consume webauthn session: %w", delErr)
	}
	return data, nil
}

func toWebAuthnCredentials(creds []WebAuthnCredential) []webauthn.Credential {
	out := make([]webauthn.Credential, 0, len(creds))
	for _, c := range creds {
		out = append(out, webauthn.Credential{
			ID:              c.CredentialID,
			PublicKey:       c.PublicKey,
			AttestationType: c.AttestationType,
			Authenticator:   webauthn.Authenticator{AAGUID: c.AAGUID, SignCount: c.SignCount, CloneWarning: c.CloneWarning},
		})
	}
	return out
}

func (s *Service) BeginRegistration(ctx context.Context, userID, userName, displayName string) (*protocol.CredentialCreation, string, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, "", errors.New("webauthn registration requires a user id")
	}
	user := &WebAuthnUser{ID: userID, Name: userName, DisplayName: displayName}
	creds, err := s.credStore.GetCredentials(ctx, userID)
	if err != nil {
		// A failed lookup would silently drop the exclude list and let the same
		// authenticator be enrolled twice.
		return nil, "", fmt.Errorf("list existing webauthn credentials: %w", err)
	}
	user.Credentials = toWebAuthnCredentials(creds)
	creation, sessionData, err := s.wa.BeginRegistration(user)
	if err != nil {
		return nil, "", err
	}
	sessionID := uuid.NewString()
	data, err := json.Marshal(sessionData)
	if err != nil {
		return nil, "", err
	}
	if err := s.sessionStore.Save(ctx, "webauthn:reg:"+sessionID, data, registrationSessionTTL); err != nil {
		return nil, "", err
	}
	return creation, sessionID, nil
}

func (s *Service) FinishRegistration(ctx context.Context, sessionID, userID string, rawBody []byte) (*webauthn.Credential, error) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(userID) == "" {
		return nil, errors.New("webauthn registration requires a session id and a user id")
	}
	if len(rawBody) == 0 {
		return nil, errors.New("webauthn registration response is empty")
	}
	data, err := s.consumeSession(ctx, "webauthn:reg:"+sessionID)
	if err != nil {
		return nil, err
	}
	var sessionData webauthn.SessionData
	if err := json.Unmarshal(data, &sessionData); err != nil {
		return nil, errors.New("stored webauthn registration session is unreadable")
	}
	// The ceremony was begun for a specific user; a session id must not be
	// usable to enrol a credential on someone else's account.
	if len(sessionData.UserID) > 0 && !bytes.Equal(sessionData.UserID, []byte(userID)) {
		return nil, errors.New("webauthn session does not belong to this user")
	}
	user := &WebAuthnUser{ID: userID}

	httpReq, err := newAssertionRequest(rawBody)
	if err != nil {
		return nil, err
	}

	credential, err := s.wa.FinishRegistration(user, sessionData, httpReq)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := s.credStore.SaveCredential(ctx, userID, WebAuthnCredential{
		ID: uuid.NewString(), UserID: userID, CredentialID: credential.ID,
		PublicKey: credential.PublicKey, AttestationType: credential.AttestationType,
		AAGUID: credential.Authenticator.AAGUID, SignCount: credential.Authenticator.SignCount,
		Name: "Security Key", CreatedAt: now, LastUsedAt: now,
	}); err != nil {
		return nil, err
	}
	return credential, nil
}

func newAssertionRequest(rawBody []byte) (*http.Request, error) {
	httpReq, err := http.NewRequest("POST", "/", io.NopCloser(bytes.NewReader(rawBody)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.ContentLength = int64(len(rawBody))
	return httpReq, nil
}

func (s *Service) BeginLogin(ctx context.Context) (*protocol.CredentialAssertion, string, error) {
	assertion, sessionData, err := s.wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, "", err
	}
	sessionID := uuid.NewString()
	data, err := json.Marshal(sessionData)
	if err != nil {
		return nil, "", err
	}
	if err := s.sessionStore.Save(ctx, "webauthn:login:"+sessionID, data, loginSessionTTL); err != nil {
		return nil, "", err
	}
	return assertion, sessionID, nil
}

func (s *Service) FinishLogin(ctx context.Context, sessionID, userID string, rawBody []byte) (*webauthn.Credential, error) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(userID) == "" {
		return nil, errors.New("webauthn login requires a session id and a user id")
	}
	if len(rawBody) == 0 {
		return nil, errors.New("webauthn login response is empty")
	}
	data, err := s.consumeSession(ctx, "webauthn:login:"+sessionID)
	if err != nil {
		return nil, err
	}
	var sessionData webauthn.SessionData
	if err := json.Unmarshal(data, &sessionData); err != nil {
		return nil, errors.New("stored webauthn login session is unreadable")
	}
	if len(sessionData.UserID) > 0 && !bytes.Equal(sessionData.UserID, []byte(userID)) {
		return nil, errors.New("webauthn session does not belong to this user")
	}

	stored, err := s.credStore.GetCredentials(ctx, userID)
	if err != nil {
		// Never treat a failed credential lookup as "no credentials": the
		// assertion below would then be rejected for the wrong reason and the
		// real outage would be hidden.
		return nil, fmt.Errorf("list webauthn credentials: %w", err)
	}
	if len(stored) == 0 {
		return nil, errors.New("user has no registered webauthn credentials")
	}
	user := &WebAuthnUser{ID: userID, Credentials: toWebAuthnCredentials(stored)}

	httpReq, err := newAssertionRequest(rawBody)
	if err != nil {
		return nil, err
	}

	credential, err := s.wa.FinishLogin(user, sessionData, httpReq)
	if err != nil {
		return nil, err
	}

	// Persist the ratcheted sign count and clone verdict for the credential that
	// actually signed, otherwise clone detection never advances.
	rowID := ""
	for _, c := range stored {
		if bytes.Equal(c.CredentialID, credential.ID) {
			rowID = c.ID
			break
		}
	}
	if rowID == "" {
		return nil, errors.New("verified webauthn credential is no longer stored")
	}
	if err := s.credStore.RecordCredentialUsage(ctx, userID, rowID,
		credential.Authenticator.SignCount, credential.Authenticator.CloneWarning, time.Now().UTC()); err != nil {
		return nil, fmt.Errorf("record webauthn credential usage: %w", err)
	}
	return credential, nil
}

func (s *Service) ListCredentials(ctx context.Context, userID string) ([]WebAuthnCredential, error) {
	return s.credStore.GetCredentials(ctx, userID)
}

func (s *Service) RemoveCredential(ctx context.Context, userID, credentialID string) error {
	return s.credStore.RemoveCredential(ctx, userID, credentialID)
}
