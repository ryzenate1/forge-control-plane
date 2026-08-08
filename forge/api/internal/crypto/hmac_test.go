package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestSign(t *testing.T) {
	t.Parallel()
	secret := "my-secret-key"
	data := []byte("hello world")

	sig, err := Sign(secret, data)
	if err != nil {
		t.Fatalf("Sign() unexpected error: %v", err)
	}
	if sig == "" {
		t.Fatal("Sign() returned empty signature")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	expected := hex.EncodeToString(mac.Sum(nil))
	if sig != expected {
		t.Fatalf("Sign() = %q, want %q", sig, expected)
	}
}

func TestSign_EmptySecret(t *testing.T) {
	t.Parallel()
	_, err := Sign("", []byte("data"))
	if err == nil {
		t.Fatal("Sign() expected error for empty secret")
	}
}

func TestSignB64(t *testing.T) {
	t.Parallel()
	secret := "my-secret-key"
	data := []byte("hello world")

	sig, err := SignB64(secret, data)
	if err != nil {
		t.Fatalf("SignB64() unexpected error: %v", err)
	}
	if sig == "" {
		t.Fatal("SignB64() returned empty signature")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if sig != expected {
		t.Fatalf("SignB64() = %q, want %q", sig, expected)
	}
}

func TestSignB64_EmptySecret(t *testing.T) {
	t.Parallel()
	_, err := SignB64("", []byte("data"))
	if err == nil {
		t.Fatal("SignB64() expected error for empty secret")
	}
}

func TestEqual_Match(t *testing.T) {
	t.Parallel()
	if !Equal("abc123", "abc123") {
		t.Fatal("Equal() = false, want true for matching strings")
	}
}

func TestEqual_NoMatch(t *testing.T) {
	t.Parallel()
	if Equal("abc123", "def456") {
		t.Fatal("Equal() = true, want false for different strings")
	}
}

func TestEqual_Empty(t *testing.T) {
	t.Parallel()
	if !Equal("", "") {
		t.Fatal("Equal() = false, want true for empty strings")
	}
}

func TestSign_Deterministic(t *testing.T) {
	t.Parallel()
	secret := "test-secret"
	data := []byte("same data")

	sig1, err := Sign(secret, data)
	if err != nil {
		t.Fatal(err)
	}
	sig2, err := Sign(secret, data)
	if err != nil {
		t.Fatal(err)
	}
	if sig1 != sig2 {
		t.Fatal("Sign() not deterministic for same inputs")
	}
}

func TestSign_DifferentSecrets(t *testing.T) {
	t.Parallel()
	data := []byte("same data")

	sig1, _ := Sign("secret-1", data)
	sig2, _ := Sign("secret-2", data)
	if sig1 == sig2 {
		t.Fatal("Sign() produced same signature for different secrets")
	}
}

func TestSignB64_Deterministic(t *testing.T) {
	t.Parallel()
	secret := "test-secret"
	data := []byte("same data")

	sig1, _ := SignB64(secret, data)
	sig2, _ := SignB64(secret, data)
	if sig1 != sig2 {
		t.Fatal("SignB64() not deterministic for same inputs")
	}
}
