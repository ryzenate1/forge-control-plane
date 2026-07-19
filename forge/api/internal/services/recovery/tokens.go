package recovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

type TokenType string

const (
	TokenPasswordReset     TokenType = "password_reset"
	TokenAccountRecovery   TokenType = "account_recovery"
	TokenEmailVerification TokenType = "email_verification"
	Token2FARecovery       TokenType = "2fa_recovery"
)

type RecoveryToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"userId"`
	Type      TokenType  `json:"type"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expiresAt"`
	UsedAt    *time.Time `json:"usedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	Metadata  string     `json:"metadata,omitempty"`
	IP        string     `json:"ip,omitempty"`
	UserAgent string     `json:"userAgent,omitempty"`
}

type TokenStore interface {
	CreateToken(ctx context.Context, token *RecoveryToken) error
	GetTokenByHash(ctx context.Context, hash string) (*RecoveryToken, error)
	MarkTokenUsed(ctx context.Context, id string) error
	InvalidateUserTokens(ctx context.Context, userID string, tokenType TokenType) error
	ListUserTokens(ctx context.Context, userID string, tokenType TokenType, limit int) ([]RecoveryToken, error)
	CleanupExpiredTokens(ctx context.Context) (int64, error)
}

type TokenService struct {
	store    TokenStore
	tokenTTL map[TokenType]time.Duration
}

func NewTokenService(store TokenStore) *TokenService {
	return &TokenService{
		store: store,
		tokenTTL: map[TokenType]time.Duration{
			TokenPasswordReset:     30 * time.Minute,
			TokenAccountRecovery:   15 * time.Minute,
			TokenEmailVerification: 24 * time.Hour,
			Token2FARecovery:       0,
		},
	}
}

func (s *TokenService) GenerateToken(ctx context.Context, userID string, tokenType TokenType, ip, userAgent, metadata string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	plaintext := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(sum[:])

	ttl := s.tokenTTL[tokenType]
	expiresAt := time.Now().UTC().Add(ttl)
	if ttl == 0 {
		expiresAt = time.Now().UTC().Add(100 * 365 * 24 * time.Hour)
	}

	token := &RecoveryToken{
		ID:        uuid.NewString(),
		UserID:    userID,
		Type:      tokenType,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
		Metadata:  metadata,
		IP:        ip,
		UserAgent: userAgent,
	}

	if err := s.store.CreateToken(ctx, token); err != nil {
		return "", err
	}

	return plaintext, nil
}

func (s *TokenService) ValidateToken(ctx context.Context, plaintext string, tokenType TokenType) (string, error) {
	sum := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(sum[:])

	token, err := s.store.GetTokenByHash(ctx, tokenHash)
	if err != nil {
		return "", err
	}

	if token.Type != tokenType {
		return "", nil
	}

	if token.UsedAt != nil {
		return "", nil
	}

	if time.Now().UTC().After(token.ExpiresAt) {
		return "", nil
	}

	return token.UserID, nil
}

func (s *TokenService) ConsumeToken(ctx context.Context, plaintext string, tokenType TokenType) (string, error) {
	userID, err := s.ValidateToken(ctx, plaintext, tokenType)
	if err != nil || userID == "" {
		return userID, err
	}

	sum := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(sum[:])

	token, err := s.store.GetTokenByHash(ctx, tokenHash)
	if err != nil {
		return "", err
	}

	if err := s.store.MarkTokenUsed(ctx, token.ID); err != nil {
		return "", err
	}

	return userID, nil
}

func (s *TokenService) InvalidateUserTokens(ctx context.Context, userID string, tokenType TokenType) error {
	return s.store.InvalidateUserTokens(ctx, userID, tokenType)
}

func (s *TokenService) GenerateRecoveryCodes(ctx context.Context, userID string, count int) ([]string, error) {
	var codes []string
	for i := 0; i < count; i++ {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		code := base64.StdEncoding.EncodeToString(raw)
		codes = append(codes, code)

		sum := sha256.Sum256([]byte(code))
		tokenHash := hex.EncodeToString(sum[:])

		token := &RecoveryToken{
			ID:        uuid.NewString(),
			UserID:    userID,
			Type:      Token2FARecovery,
			TokenHash: tokenHash,
			ExpiresAt: time.Now().UTC().Add(100 * 365 * 24 * time.Hour),
			CreatedAt: time.Now().UTC(),
			Metadata:  `{"recovery_code": true}`,
		}

		if err := s.store.CreateToken(ctx, token); err != nil {
			return nil, err
		}
	}
	return codes, nil
}
