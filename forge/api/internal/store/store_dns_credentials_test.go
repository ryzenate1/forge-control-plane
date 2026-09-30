package store

import (
	"encoding/json"
	"testing"

	"gamepanel/forge/internal/secrets"
)

func TestSecretAAD_Namespaced(t *testing.T) {
	aad := secretAAD("certificates", "id-123", "dns_credentials")
	if aad != "forge:secret:certificates:id-123:dns_credentials" {
		t.Fatalf("secretAAD unexpected: %q", aad)
	}
	legacy := secretAADLegacy("certificates", "id-123", "dns_credentials")
	if legacy != "certificates:id-123:dns_credentials" {
		t.Fatalf("legacy AAD unexpected: %q", legacy)
	}
	if aad == legacy {
		t.Fatal("namespaced and legacy AAD should differ")
	}
}

func TestEncryptDecrypt_DNSCredentials_Namespaced(t *testing.T) {
	ring, _ := secrets.New("test", "0000000000000000000000000000000000000000000000000000000000000000", nil)
	s := &Store{secrets: ring}
	creds := map[string]string{"apiKey": "secret123", "secret": "s3cr3t"}
	b, _ := json.Marshal(creds)
	aad := secretAAD("certificates", "cert-1", "dns_credentials")
	enc, err := s.encryptSecret(string(b), aad)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == "" || enc == string(b) {
		t.Fatal("encrypted should not be empty or plaintext")
	}
	// Ensure plaintext column would be cleared (dual-write)
	// Decrypt with same AAD should succeed
	dec, err := s.decryptSecret(enc, "", aad)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(dec), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["apiKey"] != "secret123" {
		t.Fatalf("decrypted mismatch: %v", out)
	}
	// Cross-resource AAD should fail
	otherAAD := secretAAD("dns_provider_accounts", "cert-1", "credentials")
	if _, err := s.secrets.Decrypt(enc, otherAAD); err == nil {
		t.Fatal("cross-resource decrypt should fail")
	}
}

func TestIsLegacyAAD_Detection(t *testing.T) {
	ring, _ := secrets.New("test", "0000000000000000000000000000000000000000000000000000000000000000", nil)
	s := &Store{secrets: ring}
	aad := secretAAD("certificates", "cid", "dns_credentials")
	legacy := secretAADLegacy("certificates", "cid", "dns_credentials")
	plaintext := `{"key":"value"}`
	// Encrypt with legacy AAD
	legacyEnc, _ := ring.Encrypt([]byte(plaintext), legacy)
	// isLegacyAAD should detect legacy envelope when checking new AAD
	if !s.isLegacyAAD(legacyEnc, aad) {
		t.Fatal("should detect legacy AAD")
	}
	// Encrypt with new AAD, should not be legacy
	newEnc, _ := ring.Encrypt([]byte(plaintext), aad)
	if s.isLegacyAAD(newEnc, aad) {
		t.Fatal("new envelope should not be detected as legacy")
	}
	// Decrypt via store fallback should succeed for legacy
	dec, err := s.decryptSecret(legacyEnc, "", aad)
	if err != nil || dec != plaintext {
		t.Fatalf("fallback decrypt failed: %v %q", err, dec)
	}
}

func TestDNSProviderAccount_CredentialsEncryption(t *testing.T) {
	ring, _ := secrets.New("test", "0000000000000000000000000000000000000000000000000000000000000000", nil)
	s := &Store{secrets: ring}
	raw := json.RawMessage(`{"token":"abc123"}`)
	aad := secretAAD("dns_provider_accounts", "prov-1", "credentials")
	enc, err := s.encryptSecret(string(raw), aad)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := s.decryptSecret(enc, "", aad)
	if err != nil {
		t.Fatal(err)
	}
	if dec != string(raw) {
		t.Fatalf("decrypted mismatch: got %q want %q", dec, string(raw))
	}
	// Ensure isLegacy detection works for DNS as well
	legacy := secretAADLegacy("dns_provider_accounts", "prov-1", "credentials")
	legacyEnc, _ := ring.Encrypt([]byte(string(raw)), legacy)
	if !s.isLegacyAAD(legacyEnc, aad) {
		t.Fatal("DNS legacy should be detected")
	}
	// Fallback decrypt should succeed
	dec2, err := s.decryptSecret(legacyEnc, "", aad)
	if err != nil || dec2 != string(raw) {
		t.Fatalf("fallback DNS decrypt failed: %v %q", err, dec2)
	}
}
