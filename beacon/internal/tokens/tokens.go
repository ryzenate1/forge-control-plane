package tokens

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Scope string

const (
	ScopeWebsocket      Scope = "websocket"
	ScopeFileDownload   Scope = "file-download"
	ScopeBackupDownload Scope = "backup-download"
	ScopeFileUpload     Scope = "file-upload"
	ScopeTransfer       Scope = "transfer"
)

type Claims struct {
	Scope     Scope     `json:"scope"`
	ServerID  string    `json:"server_id"`
	User      string    `json:"user,omitempty"`
	FilePath  string    `json:"file_path,omitempty"`
	BackupID  string    `json:"backup_id,omitempty"`
	UniqueID  string    `json:"unique_id,omitempty"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

var (
	ErrInvalidToken    = errors.New("invalid token")
	ErrTokenExpired    = errors.New("token has expired")
	ErrInvalidScope    = errors.New("invalid token scope")
	ErrInvalidSignature = errors.New("invalid token signature")
)

type Generator struct {
	secret []byte
}

func NewGenerator(secret []byte) *Generator {
	return &Generator{secret: secret}
}

func (g *Generator) Generate(claims Claims) (string, error) {
	if claims.IssuedAt.IsZero() {
		claims.IssuedAt = time.Now()
	}

	header := jwtHeader{Alg: "HS256", Typ: "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal header: %w", err)
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %w", err)
	}

	headerEnc := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsEnc := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := headerEnc + "." + claimsEnc
	signature := g.sign([]byte(signingInput))
	signatureEnc := base64.RawURLEncoding.EncodeToString(signature)

	return signingInput + "." + signatureEnc, nil
}

func (g *Generator) Validate(tokenString string) (*Claims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}

	expectedSig := g.sign([]byte(signingInput))
	if !hmac.Equal(signature, expectedSig) {
		return nil, ErrInvalidSignature
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if !claims.ExpiresAt.IsZero() && time.Now().After(claims.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	return &claims, nil
}

func (g *Generator) sign(data []byte) []byte {
	mac := hmac.New(sha256.New, g.secret)
	mac.Write(data)
	return mac.Sum(nil)
}

func (g *Generator) GenerateFileDownload(serverID, filePath, user string, ttl time.Duration) (string, error) {
	return g.Generate(Claims{
		Scope:     ScopeFileDownload,
		ServerID:  serverID,
		User:      user,
		FilePath:  filePath,
		UniqueID:  uuid.New().String(),
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	})
}

func (g *Generator) GenerateBackupDownload(serverID, backupID, user string, ttl time.Duration) (string, error) {
	return g.Generate(Claims{
		Scope:     ScopeBackupDownload,
		ServerID:  serverID,
		User:      user,
		BackupID:  backupID,
		UniqueID:  uuid.New().String(),
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	})
}

func (g *Generator) GenerateUpload(serverID, user string, ttl time.Duration) (string, error) {
	return g.Generate(Claims{
		Scope:     ScopeFileUpload,
		ServerID:  serverID,
		User:      user,
		UniqueID:  uuid.New().String(),
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	})
}

func (g *Generator) GenerateWebsocket(serverID, user string, ttl time.Duration) (string, error) {
	return g.Generate(Claims{
		Scope:     ScopeWebsocket,
		ServerID:  serverID,
		User:      user,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	})
}
