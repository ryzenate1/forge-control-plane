package daemon

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Beacon scoped-token minting (panel side).
//
// The beacon daemon authenticates websocket upgrades with short-lived,
// scope-bound HMAC-SHA256 tokens signed with the node's shared secret
// (beacon/internal/tokens). This file is the panel-side counterpart that
// mints those tokens immediately before proxying a browser websocket to a
// node, so the panel→beacon hop carries least-privilege credentials instead
// of long-lived node tokens.
//
// Contract (must stay byte-compatible with beacon/internal/tokens):
//   - header: {"alg":"HS256","typ":"JWT"}, base64url (raw, no padding)
//   - claims: scope/server_id/user/unique_id/iat/exp (exp enforced by beacon)
//   - signature: HMAC-SHA256 over "header.claims" with the node token

type beaconJWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type beaconWSClaims struct {
	Scope     string    `json:"scope"`
	ServerID  string    `json:"server_id"`
	User      string    `json:"user,omitempty"`
	UniqueID  string    `json:"unique_id,omitempty"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
}

const defaultWebsocketTokenTTL = 90 * time.Second

func signBeaconToken(secret []byte, claims beaconWSClaims) (string, error) {
	if len(secret) == 0 {
		return "", fmt.Errorf("node token must not be empty")
	}
	headerJSON, err := json.Marshal(beaconJWTHeader{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", fmt.Errorf("marshal jwt header: %w", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal jwt claims: %w", err)
	}
	headerEnc := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsEnc := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerEnc + "." + claimsEnc
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	sig := mac.Sum(nil)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// MintWebsocketToken returns a beacon websocket-scoped token for serverID.
// The token grants stream access only (console/stats/logs); it never implies
// install or admin authority on the node. TTL defaults to 90s and is only
// carried over the direct panel→beacon connection.
func MintWebsocketToken(nodeToken, serverID, user string) (string, error) {
	return signBeaconToken([]byte(nodeToken), beaconWSClaims{
		Scope:     "websocket",
		ServerID:  serverID,
		User:      user,
		UniqueID:  uuid.New().String(),
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(defaultWebsocketTokenTTL),
	})
}

// MintAdminToken returns a beacon admin-scoped token for privileged
// operations such as install streaming (GET /servers/:id/install/ws).
// Only admin-scoped tokens are accepted by beacon's installWS handler;
// websocket-scoped tokens are explicitly rejected to prevent tenant
// privilege escalation. TTL defaults to 90s.
func MintAdminToken(nodeToken, serverID, user string) (string, error) {
	return signBeaconToken([]byte(nodeToken), beaconWSClaims{
		Scope:     "admin",
		ServerID:  serverID,
		User:      user,
		UniqueID:  uuid.New().String(),
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(defaultWebsocketTokenTTL),
	})
}
