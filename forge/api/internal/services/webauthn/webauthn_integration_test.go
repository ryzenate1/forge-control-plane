package webauthn

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu    sync.Mutex
	creds map[string][]WebAuthnCredential
}

func (m *memoryStore) GetCredentials(ctx context.Context, userID string) ([]WebAuthnCredential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.creds[userID], nil
}

func (m *memoryStore) SaveCredential(ctx context.Context, userID string, cred WebAuthnCredential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.creds[userID] = append(m.creds[userID], cred)
	return nil
}

func (m *memoryStore) RemoveCredential(ctx context.Context, userID, credentialID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	creds := m.creds[userID]
	for i, c := range creds {
		if c.ID == credentialID {
			m.creds[userID] = append(creds[:i], creds[i+1:]...)
			break
		}
	}
	return nil
}

type memSessionStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (m *memSessionStore) Save(ctx context.Context, key string, data []byte, expiry time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = data
	return nil
}

func (m *memSessionStore) Get(ctx context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[key], nil
}

func (m *memSessionStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func TestWebAuthnServiceIntegration(t *testing.T) {
	credStore := &memoryStore{creds: make(map[string][]WebAuthnCredential)}
	sessionStore := &memSessionStore{data: make(map[string][]byte)}

	svc, err := New("localhost", "GamePanel", "http://localhost:3000", credStore, sessionStore)
	if err != nil {
		t.Fatal(err)
	}

	creation, sessionID, err := svc.BeginRegistration(context.Background(), "user-1", "test@test.com", "Test User")
	if err != nil {
		t.Fatal(err)
	}

	if creation == nil {
		t.Fatal("expected non-nil creation")
	}
	if sessionID == "" {
		t.Fatal("expected non-empty session ID")
	}

	assertion, loginSessionID, err := svc.BeginLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if assertion == nil {
		t.Fatal("expected non-nil assertion")
	}
	if loginSessionID == "" {
		t.Fatal("expected non-empty login session ID")
	}

	creds, err := credStore.GetCredentials(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 0 {
		t.Errorf("expected 0 creds, got %d", len(creds))
	}
}
