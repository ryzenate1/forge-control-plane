package daemon

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Panel-side verification of the beacon scoped-token wire contract. The
// beacon's own verifier is exercised in beacon/internal/tokens tests via a
// golden vector produced with this same canonical algorithm (see
// TestPanelMintedWebsocketTokenGoldenVector there). Together they pin both
// directions of the panel↔beacon websocket trust contract.

func mintForTest(t *testing.T, secret, serverID, user string, iat time.Time) string {
	t.Helper()
	token, err := signBeaconToken([]byte(secret), beaconWSClaims{
		Scope:     "websocket",
		ServerID:  serverID,
		User:      user,
		UniqueID:  "fixed-unique-id",
		IssuedAt:  iat,
		ExpiresAt: iat.Add(defaultWebsocketTokenTTL),
	})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return token
}

func decodeSegment(t *testing.T, seg string) []byte {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		t.Fatalf("decode segment: %v", err)
	}
	return raw
}

func TestMintedTokenStructure(t *testing.T) {
	iat := time.Unix(1700000000, 0)
	token := mintForTest(t, "secret-0123456789abcdef", "srv-42", "user-7", iat)

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3-part JWT, got %d parts", len(parts))
	}

	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(decodeSegment(t, parts[0]), &header); err != nil {
		t.Fatalf("header: %v", err)
	}
	if header.Alg != "HS256" || header.Typ != "JWT" {
		t.Errorf("unexpected header %+v", header)
	}

	var claims map[string]any
	if err := json.Unmarshal(decodeSegment(t, parts[1]), &claims); err != nil {
		t.Fatalf("claims: %v", err)
	}
	for _, key := range []string{"scope", "server_id", "iat", "exp"} {
		if _, ok := claims[key]; !ok {
			t.Errorf("missing required claim key %q in %v", key, claims)
		}
	}
	if claims["scope"] != "websocket" {
		t.Errorf("scope = %v, want websocket", claims["scope"])
	}
	if claims["server_id"] != "srv-42" {
		t.Errorf("server_id = %v, want srv-42", claims["server_id"])
	}

	// Signature must be HMAC-SHA256 over "header.claims".
	mac := hmac.New(sha256.New, []byte("secret-0123456789abcdef"))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	wantSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if wantSig != parts[2] {
		t.Error("signature does not match HMAC-SHA256 canonical construction")
	}
}

func TestMintWebsocketTokenDefaults(t *testing.T) {
	before := time.Now()
	token, err := MintWebsocketToken("k-0123456789abcdef", "srv-1", "u-1")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	parts := strings.Split(token, ".")
	var claims struct {
		Scope     string    `json:"scope"`
		ServerID  string    `json:"server_id"`
		User      string    `json:"user"`
		ExpiresAt time.Time `json:"exp"`
	}
	if err := json.Unmarshal(decodeSegment(t, parts[1]), &claims); err != nil {
		t.Fatalf("claims: %v", err)
	}
	if claims.Scope != "websocket" || claims.ServerID != "srv-1" || claims.User != "u-1" {
		t.Errorf("unexpected claims %+v", claims)
	}
	ttl := claims.ExpiresAt.Sub(before)
	if ttl <= 0 || ttl > 2*time.Minute {
		t.Errorf("websocket token TTL out of bounds: %v", ttl)
	}
}

func TestMintWebsocketTokenEmptyNodeToken(t *testing.T) {
	if _, err := MintWebsocketToken("", "srv-1", "u"); err == nil {
		t.Fatal("expected error for empty node token")
	}
}
