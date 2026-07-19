package recovery

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockTokenStore struct {
	mu     sync.Mutex
	tokens map[string]*RecoveryToken
	byHash map[string]*RecoveryToken
}

func (m *mockTokenStore) CreateToken(ctx context.Context, token *RecoveryToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[token.ID] = token
	m.byHash[token.TokenHash] = token
	return nil
}

func (m *mockTokenStore) GetTokenByHash(ctx context.Context, hash string) (*RecoveryToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	token, ok := m.byHash[hash]
	if !ok {
		return nil, nil
	}
	return token, nil
}

func (m *mockTokenStore) MarkTokenUsed(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	if token, ok := m.tokens[id]; ok {
		token.UsedAt = &now
	}
	return nil
}

func (m *mockTokenStore) InvalidateUserTokens(ctx context.Context, userID string, tokenType TokenType) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, token := range m.tokens {
		if token.UserID == userID && token.Type == tokenType {
			now := time.Now().UTC()
			token.UsedAt = &now
		}
	}
	return nil
}

func (m *mockTokenStore) ListUserTokens(ctx context.Context, userID string, tokenType TokenType, limit int) ([]RecoveryToken, error) {
	return nil, nil
}

func (m *mockTokenStore) CleanupExpiredTokens(ctx context.Context) (int64, error) {
	return 0, nil
}

func TestGenerateAndValidateToken(t *testing.T) {
	store := &mockTokenStore{
		tokens: make(map[string]*RecoveryToken),
		byHash: make(map[string]*RecoveryToken),
	}
	svc := NewTokenService(store)

	plaintext, err := svc.GenerateToken(context.Background(), "user-1", TokenPasswordReset, "127.0.0.1", "test-agent", "")
	if err != nil {
		t.Fatal(err)
	}

	if plaintext == "" {
		t.Fatal("expected non-empty token")
	}

	userID, err := svc.ValidateToken(context.Background(), plaintext, TokenPasswordReset)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "user-1" {
		t.Errorf("expected userID 'user-1', got %q", userID)
	}

	userID, err = svc.ConsumeToken(context.Background(), plaintext, TokenPasswordReset)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "user-1" {
		t.Errorf("expected userID 'user-1', got %q", userID)
	}

	userID, err = svc.ValidateToken(context.Background(), plaintext, TokenPasswordReset)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "" {
		t.Error("expected empty userID for consumed token")
	}
}

func TestWrongTokenType(t *testing.T) {
	store := &mockTokenStore{
		tokens: make(map[string]*RecoveryToken),
		byHash: make(map[string]*RecoveryToken),
	}
	svc := NewTokenService(store)

	plaintext, _ := svc.GenerateToken(context.Background(), "user-1", TokenPasswordReset, "", "", "")

	userID, err := svc.ValidateToken(context.Background(), plaintext, TokenAccountRecovery)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "" {
		t.Error("expected empty userID for wrong token type")
	}
}

func TestGenerateRecoveryCodes(t *testing.T) {
	store := &mockTokenStore{
		tokens: make(map[string]*RecoveryToken),
		byHash: make(map[string]*RecoveryToken),
	}
	svc := NewTokenService(store)

	codes, err := svc.GenerateRecoveryCodes(context.Background(), "user-1", 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(codes) != 10 {
		t.Errorf("expected 10 codes, got %d", len(codes))
	}

	if len(store.tokens) != 10 {
		t.Errorf("expected 10 stored tokens, got %d", len(store.tokens))
	}
}
