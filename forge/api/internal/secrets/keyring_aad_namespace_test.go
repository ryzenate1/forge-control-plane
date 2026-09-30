package secrets

import (
	"strings"
	"testing"
)

func TestAADNamespace_IsolatedPerResource(t *testing.T) {
	ring, err := New("test", "0000000000000000000000000000000000000000000000000000000000000000", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate store's secretAAD namespacing: "forge:secret:table:id:field"
	aad1 := "forge:secret:certificates:id1:dns_credentials"
	aad2 := "forge:secret:dns_provider_accounts:id1:credentials"
	plaintext := "secret-data"

	env1, err := ring.Encrypt([]byte(plaintext), aad1)
	if err != nil {
		t.Fatal(err)
	}
	// Decrypt with same AAD should succeed
	if dec, err := ring.Decrypt(env1, aad1); err != nil || string(dec) != plaintext {
		t.Fatalf("decrypt with same AAD failed: %v %q", err, dec)
	}
	// Decrypt with different resource AAD should fail (GCM auth)
	if _, err := ring.Decrypt(env1, aad2); err == nil {
		t.Fatal("decrypt with different resource AAD should fail")
	}
	// Legacy AAD (without prefix) should not decrypt new envelope
	legacy := strings.TrimPrefix(aad1, "forge:secret:")
	if _, err := ring.Decrypt(env1, legacy); err == nil {
		t.Fatal("new envelope should not decrypt with legacy AAD")
	}

	// Legacy envelope should decrypt with legacy AAD but not new
	legacyAAD := "certificates:id2:private_key"
	envLegacy, err := ring.Encrypt([]byte(plaintext), legacyAAD)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ring.Decrypt(envLegacy, legacyAAD); err != nil {
		t.Fatalf("legacy decrypt failed: %v", err)
	}
	if _, err := ring.Decrypt(envLegacy, "forge:secret:"+legacyAAD); err == nil {
		t.Fatal("legacy envelope should not decrypt with new namespaced AAD")
	}
	// Verify namespacing prevents cross-table substitution
	crossAAD := "forge:secret:certificates:id1:private_key"
	if _, err := ring.Decrypt(env1, crossAAD); err == nil {
		t.Fatal("cross-field AAD should not decrypt")
	}
}

func TestParseKey_HexVsBase64Ambiguity(t *testing.T) {
	// 64-char hex string should be parsed as hex, not base64
	hexKey := strings.Repeat("ab", 32) // 64 hex chars
	if _, err := ParseKey(hexKey); err != nil {
		t.Fatalf("valid hex key should parse: %v", err)
	}
	testKey := "0000000000000000000000000000000000000000000000000000000000000000"
	// This is 64 hex chars of zeros, should parse as hex
	if _, err := ParseKey(testKey); err != nil {
		t.Fatalf("hex zeros should parse: %v", err)
	}
	_ = hexKey
}
