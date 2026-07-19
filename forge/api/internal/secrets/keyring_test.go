package secrets

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func testKey(fill byte) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Repeat(string(fill), 32)))
}

func TestParseKeyRejectsInvalidKeys(t *testing.T) {
	for _, value := range []string{"", "short", base64.StdEncoding.EncodeToString(make([]byte, 31)), strings.Repeat("z", 64)} {
		if _, err := ParseKey(value); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("ParseKey(%q) error = %v", value, err)
		}
	}
	if _, err := ParseKey(strings.Repeat("ab", 32)); err != nil {
		t.Fatalf("valid hex key: %v", err)
	}
}

func TestRoundTripTamperAndNonceUniqueness(t *testing.T) {
	ring, err := New("current", testKey('a'), nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ring.Encrypt([]byte("secret"), "nodes:id:daemon_token")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ring.Encrypt([]byte("secret"), "nodes:id:daemon_token")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("AES-GCM envelopes reused a nonce")
	}
	plaintext, err := ring.Decrypt(first, "nodes:id:daemon_token")
	if err != nil || string(plaintext) != "secret" {
		t.Fatalf("decrypt = %q, %v", plaintext, err)
	}
	tampered := first[:len(first)-1] + "A"
	if tampered == first {
		tampered = first[:len(first)-1] + "B"
	}
	if _, err := ring.Decrypt(tampered, "nodes:id:daemon_token"); err == nil {
		t.Fatal("tampered ciphertext decrypted")
	}
	if _, err := ring.Decrypt(first, "nodes:other:daemon_token"); err == nil {
		t.Fatal("ciphertext decrypted under different AAD")
	}
}

func TestPreviousKeyDecryptAndActiveKeyReencrypt(t *testing.T) {
	old, err := New("old", testKey('o'), nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := old.Encrypt([]byte("rotate-me"), "field")
	if err != nil {
		t.Fatal(err)
	}
	current, err := New("current", testKey('n'), map[string]string{"old": testKey('o')})
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := current.Decrypt(envelope, "field")
	if err != nil || string(plaintext) != "rotate-me" {
		t.Fatalf("previous decrypt = %q, %v", plaintext, err)
	}
	if !current.NeedsRotation(envelope) {
		t.Fatal("old envelope was not marked for rotation")
	}
	rotated, err := current.Encrypt(plaintext, "field")
	if err != nil {
		t.Fatal(err)
	}
	if current.NeedsRotation(rotated) || !strings.HasPrefix(rotated, "forge:v1:current:") {
		t.Fatalf("unexpected rotated envelope %q", rotated)
	}
}
