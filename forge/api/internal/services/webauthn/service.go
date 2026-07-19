package webauthn

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
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
}

type SessionStore interface {
	Save(ctx context.Context, key string, data []byte, expiry time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

type Service struct {
	wa           *webauthn.WebAuthn
	credStore    CredentialStore
	sessionStore SessionStore
}

func New(rpID, rpDisplayName, rpOrigin string, credStore CredentialStore, sessionStore SessionStore) (*Service, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: rpDisplayName,
		RPID:          rpID,
		RPOrigins:     []string{rpOrigin},
	})
	if err != nil {
		return nil, err
	}
	return &Service{wa: wa, credStore: credStore, sessionStore: sessionStore}, nil
}

func (s *Service) BeginRegistration(ctx context.Context, userID, userName, displayName string) (*protocol.CredentialCreation, string, error) {
	user := &WebAuthnUser{ID: userID, Name: userName, DisplayName: displayName}
	creds, _ := s.credStore.GetCredentials(ctx, userID)
	for _, c := range creds {
		user.Credentials = append(user.Credentials, webauthn.Credential{
			ID: c.CredentialID, PublicKey: c.PublicKey,
			AttestationType: c.AttestationType,
			Authenticator:   webauthn.Authenticator{AAGUID: c.AAGUID, SignCount: c.SignCount},
		})
	}
	creation, sessionData, err := s.wa.BeginRegistration(user)
	if err != nil {
		return nil, "", err
	}
	sessionID := uuid.NewString()
	data, _ := json.Marshal(sessionData)
	s.sessionStore.Save(ctx, "webauthn:reg:"+sessionID, data, 5*time.Minute)
	return creation, sessionID, nil
}

func (s *Service) FinishRegistration(ctx context.Context, sessionID, userID string, rawBody []byte) (*webauthn.Credential, error) {
	data, err := s.sessionStore.Get(ctx, "webauthn:reg:"+sessionID)
	if err != nil {
		return nil, err
	}
	var sessionData webauthn.SessionData
	json.Unmarshal(data, &sessionData)
	user := &WebAuthnUser{ID: userID}

	httpReq, err := http.NewRequest("POST", "/", io.NopCloser(bytes.NewReader(rawBody)))
	if err != nil {
		return nil, err
	}

	credential, err := s.wa.FinishRegistration(user, sessionData, httpReq)
	if err != nil {
		return nil, err
	}
	s.credStore.SaveCredential(ctx, userID, WebAuthnCredential{
		ID: uuid.NewString(), UserID: userID, CredentialID: credential.ID,
		PublicKey: credential.PublicKey, AttestationType: credential.AttestationType,
		AAGUID: credential.Authenticator.AAGUID, SignCount: credential.Authenticator.SignCount,
		Name: "Security Key", CreatedAt: time.Now().UTC(), LastUsedAt: time.Now().UTC(),
	})
	s.sessionStore.Delete(ctx, "webauthn:reg:"+sessionID)
	return credential, nil
}

func (s *Service) BeginLogin(ctx context.Context) (*protocol.CredentialAssertion, string, error) {
	assertion, sessionData, err := s.wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, "", err
	}
	sessionID := uuid.NewString()
	data, _ := json.Marshal(sessionData)
	s.sessionStore.Save(ctx, "webauthn:login:"+sessionID, data, 5*time.Minute)
	return assertion, sessionID, nil
}

func (s *Service) FinishLogin(ctx context.Context, sessionID, userID string, rawBody []byte) (*webauthn.Credential, error) {
	data, err := s.sessionStore.Get(ctx, "webauthn:login:"+sessionID)
	if err != nil {
		return nil, err
	}
	var sessionData webauthn.SessionData
	json.Unmarshal(data, &sessionData)

	creds, _ := s.credStore.GetCredentials(ctx, userID)
	user := &WebAuthnUser{ID: userID}
	for _, c := range creds {
		user.Credentials = append(user.Credentials, webauthn.Credential{
			ID: c.CredentialID, PublicKey: c.PublicKey,
			Authenticator: webauthn.Authenticator{SignCount: c.SignCount},
		})
	}

	httpReq, err := http.NewRequest("POST", "/", io.NopCloser(bytes.NewReader(rawBody)))
	if err != nil {
		return nil, err
	}

	credential, err := s.wa.FinishLogin(user, sessionData, httpReq)
	if err != nil {
		return nil, err
	}
	s.sessionStore.Delete(ctx, "webauthn:login:"+sessionID)
	return credential, nil
}

func (s *Service) ListCredentials(ctx context.Context, userID string) ([]WebAuthnCredential, error) {
	return s.credStore.GetCredentials(ctx, userID)
}

func (s *Service) RemoveCredential(ctx context.Context, userID, credentialID string) error {
	return s.credStore.RemoveCredential(ctx, userID, credentialID)
}
