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
	// ScopeAdmin authorizes node-wide administrative operations. It is never
	// handed to a tenant: the scopes below are bound to a single server and
	// grant stream or file access only.
	ScopeAdmin Scope = "admin"

	ScopeWebsocket      Scope = "websocket"
	ScopeFileDownload   Scope = "file-download"
	ScopeBackupDownload Scope = "backup-download"
	ScopeFileUpload     Scope = "file-upload"
	ScopeTransfer       Scope = "transfer"
)

// signingAlgorithm is the only accepted token algorithm. The header is read
// from an untrusted string, so anything that is not exactly this value -
// including "none", "HS512" or an attacker-chosen method - is rejected before
// any key material is used.
const signingAlgorithm = "HS256"

// maxIssuedSkew tolerates clock drift between the panel and this node when
// reading a token's iat. Larger future drift is rejected.
const maxIssuedSkew = 30 * time.Second

// absoluteMaxLifetime rejects tokens whose validity window is so long they
// are effectively permanent credentials. Every mint path clamps below this,
// so hitting it means the token was built by hand or is a leaked long-lived
// key being replayed.
const absoluteMaxLifetime = 12 * time.Hour

// scopeMaxLifetime bounds how long each purpose-bound token may live. A
// websocket ticket is a single upgrade credential, not a session.
var scopeMaxLifetime = map[Scope]time.Duration{
	ScopeWebsocket:      2 * time.Minute,
	ScopeFileDownload:   15 * time.Minute,
	ScopeBackupDownload: 15 * time.Minute,
	ScopeFileUpload:     15 * time.Minute,
	ScopeTransfer:       15 * time.Minute,
	ScopeAdmin:          5 * time.Minute,
}

// resourceBoundScopes must name exactly one server. An empty ServerID on one
// of these is a wildcard: it would be accepted by every per-server handler
// that only compares non-empty claims, so it is refused at validation.
var resourceBoundScopes = map[Scope]bool{
	ScopeWebsocket:      true,
	ScopeFileDownload:   true,
	ScopeBackupDownload: true,
	ScopeFileUpload:     true,
	ScopeTransfer:       true,
}

// Known reports whether the scope is one this daemon issues.
func (s Scope) Known() bool {
	switch s {
	case ScopeAdmin, ScopeWebsocket, ScopeFileDownload, ScopeBackupDownload, ScopeFileUpload, ScopeTransfer:
		return true
	default:
		return false
	}
}

// ResourceBound reports whether the scope must carry a single server binding.
func (s Scope) ResourceBound() bool { return resourceBoundScopes[s] }

// RequiresUser reports whether the scope must name the principal that may use
// it. Streams and file access are attributed actions; without a user they
// cannot be revoked when that user is deauthorized.
func (s Scope) RequiresUser() bool {
	switch s {
	case ScopeWebsocket, ScopeFileDownload, ScopeBackupDownload, ScopeFileUpload, ScopeTransfer:
		return true
	default:
		return false
	}
}

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
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token has expired")
	ErrInvalidScope       = errors.New("invalid token scope")
	ErrInvalidSignature   = errors.New("invalid token signature")
	ErrUnsupportedAlg     = errors.New("unsupported token algorithm")
	ErrLifetimeExceeded   = errors.New("token lifetime exceeds the allowed maximum")
	ErrNotYetValid        = errors.New("token issued in the future")
	ErrUnboundToken       = errors.New("token is not bound to a single resource")
	ErrTicketUnknown      = errors.New("token is not a registered one-time ticket")
	ErrTicketRedeemed     = errors.New("token has already been redeemed")
	ErrMissingCredentials = errors.New("token is missing its signing secret")
)

// Generator signs and verifies scoped tokens using an HMAC-SHA256 secret.
//
// The secret is held in memory as a plain []byte for the lifetime of the
// process. This is a best-effort-only protection: a co-located attacker
// with sufficient privilege (e.g. root, or ptrace access to this process)
// can read the secret out of /proc/<pid>/mem regardless of anything this
// package does, and Go's garbage collector does not promptly or reliably
// zero freed memory. There is no way to fully eliminate this risk from
// within a long-lived Go process holding a secret.
//
// Compensating controls should be applied at the OS/deployment level:
//   - mount /proc with hidepid=2 so only the owning user can see process
//     memory maps of this process;
//   - restrict ptrace via the Linux YAMA security module
//     (kernel.yama.ptrace_scope >= 1, ideally 2 or 3);
//   - run the process in a container or namespace with no other
//     co-tenants that could plausibly gain ptrace/proc access.
type Generator struct {
	secret []byte

	// tickets, when wired, turns purpose-bound tokens into one-time
	// credentials that Redeem consumes.
	tickets *TokenStore
}

// Zero overwrites the in-memory secret bytes with zeros in place. Callers
// that are discarding a Generator (e.g. during secret rotation) may call
// this to reduce the window during which the old secret remains readable
// in this process's memory. It is not called automatically, since the
// Generator remains unusable for signing/validation afterward.
func (g *Generator) Zero() {
	for i := range g.secret {
		g.secret[i] = 0
	}
}

func NewGenerator(secret []byte) *Generator {
	owned := append([]byte(nil), secret...)
	return &Generator{secret: owned}
}

// SetTicketStore wires a store that records minted one-time tickets. Once
// wired, Generate registers the ticket and Redeem is the only way to spend
// it; Validate keeps accepting the token so a handler can inspect claims
// before the upgrade completes. A nil store disables the mechanism.
func (g *Generator) SetTicketStore(store *TokenStore) {
	if g == nil {
		return
	}
	g.tickets = store
}

// TicketStore returns the wired one-time ticket store, if any.
func (g *Generator) TicketStore() *TokenStore {
	if g == nil {
		return nil
	}
	return g.tickets
}

func (g *Generator) Generate(claims Claims) (string, error) {
	if g == nil || len(g.secret) == 0 {
		return "", ErrMissingCredentials
	}
	if !claims.Scope.Known() {
		return "", fmt.Errorf("%w: %q", ErrInvalidScope, claims.Scope)
	}
	if claims.IssuedAt.IsZero() {
		claims.IssuedAt = time.Now()
	}
	// Clamp the lifetime instead of honouring an over-long request: a caller
	// that asks for a year must not get a year.
	if ceiling, ok := scopeMaxLifetime[claims.Scope]; ok {
		if claims.ExpiresAt.IsZero() {
			claims.ExpiresAt = claims.IssuedAt.Add(ceiling)
		}
		if limit := claims.IssuedAt.Add(ceiling); claims.ExpiresAt.After(limit) {
			claims.ExpiresAt = limit
		}
	}
	if err := validateClaims(&claims); err != nil {
		return "", err
	}
	if claims.UniqueID == "" {
		claims.UniqueID = uuid.New().String()
	}
	if claims.Scope != ScopeAdmin && g.tickets != nil {
		g.tickets.Add(claims.UniqueID, claims.ExpiresAt)
	}

	header := jwtHeader{Alg: signingAlgorithm, Typ: "JWT"}
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
	if g == nil || len(g.secret) == 0 {
		return nil, ErrMissingCredentials
	}
	parts := strings.Split(strings.TrimSpace(tokenString), ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
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

	// The header is only trusted after the signature over it verified, but
	// the algorithm is pinned regardless: a token must not be able to pick
	// how it is checked.
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var header jwtHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, ErrInvalidToken
	}
	if header.Alg != signingAlgorithm {
		return nil, ErrUnsupportedAlg
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, ErrInvalidToken
	}
	if err := validateClaims(&claims); err != nil {
		return nil, err
	}

	now := time.Now()
	if claims.ExpiresAt.IsZero() || !now.Before(claims.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	if claims.IssuedAt.After(now.Add(maxIssuedSkew)) {
		return nil, ErrNotYetValid
	}
	if lifetime := claims.ExpiresAt.Sub(claims.IssuedAt); lifetime > absoluteMaxLifetime {
		return nil, ErrLifetimeExceeded
	}
	if g.tickets != nil && claims.Scope.ResourceBound() && g.tickets.Redeemed(claims.UniqueID) {
		return nil, ErrTicketRedeemed
	}

	return &claims, nil
}

// validateClaims enforces the structural bindings every token must carry.
// It runs on both mint and validate so a token cannot be built here with a
// missing binding, and a token built elsewhere is still refused.
func validateClaims(claims *Claims) error {
	if !claims.Scope.Known() {
		return fmt.Errorf("%w: %q", ErrInvalidScope, claims.Scope)
	}
	if strings.TrimSpace(string(claims.Scope)) != string(claims.Scope) {
		return fmt.Errorf("%w: untrimmed scope", ErrInvalidScope)
	}
	if claims.Scope.ResourceBound() && strings.TrimSpace(claims.ServerID) == "" {
		return ErrUnboundToken
	}
	switch claims.Scope {
	case ScopeBackupDownload:
		if strings.TrimSpace(claims.BackupID) == "" {
			return fmt.Errorf("%w: backup download without a backup id", ErrUnboundToken)
		}
	case ScopeFileDownload:
		if strings.TrimSpace(claims.FilePath) == "" {
			return fmt.Errorf("%w: file download without a path", ErrUnboundToken)
		}
	}
	return nil
}

// Redeem validates a purpose-bound token and, when one-time tickets are
// wired, atomically spends it. Call this at the point of use (websocket
// upgrade, file download), never on a probe: a spent ticket cannot be
// re-read. Admin tokens are not one-time and are rejected here so a caller
// cannot confuse the two credential classes.
func (g *Generator) Redeem(tokenString string) (*Claims, error) {
	claims, err := g.Validate(tokenString)
	if err != nil {
		return nil, err
	}
	if !claims.Scope.ResourceBound() {
		return nil, fmt.Errorf("%w: %s is not a one-time ticket", ErrInvalidScope, claims.Scope)
	}
	// A ticket that cannot be attributed to a principal cannot be revoked
	// when that principal is deauthorized, so it is not spendable.
	if claims.Scope.RequiresUser() && strings.TrimSpace(claims.User) == "" {
		return nil, fmt.Errorf("%w: ticket has no principal", ErrUnboundToken)
	}
	if strings.TrimSpace(claims.UniqueID) == "" {
		return nil, ErrTicketUnknown
	}
	if g.tickets == nil {
		return claims, nil
	}
	if !g.tickets.Consume(claims.UniqueID) {
		return nil, ErrTicketRedeemed
	}
	return claims, nil
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

// GenerateWebsocket mints a stream ticket bound to one server and one user.
// The unique id is always present so the ticket can be spent once and
// revoked; the ttl is clamped to the websocket ceiling.
func (g *Generator) GenerateWebsocket(serverID, user string, ttl time.Duration) (string, error) {
	return g.Generate(Claims{
		Scope:     ScopeWebsocket,
		ServerID:  serverID,
		User:      user,
		UniqueID:  uuid.New().String(),
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	})
}
