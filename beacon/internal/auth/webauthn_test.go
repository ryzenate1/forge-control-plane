package auth

import (
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestNewWebAuthn(t *testing.T) {
	rpID := "example.com"
	rpOrigin := "https://example.com"
	rpDisplayName := "Example"

	webAuthn, err := NewWebAuthn(rpID, rpOrigin, rpDisplayName)
	if err != nil {
		t.Fatalf("NewWebAuthn() error = %v", err)
	}

	if webAuthn.rpID != rpID {
		t.Errorf("NewWebAuthn() rpID = %v, want %v", webAuthn.rpID, rpID)
	}
	if webAuthn.rpOrigin != rpOrigin {
		t.Errorf("NewWebAuthn() rpOrigin = %v, want %v", webAuthn.rpOrigin, rpOrigin)
	}
	if webAuthn.rpDisplayName != rpDisplayName {
		t.Errorf("NewWebAuthn() rpDisplayName = %v, want %v", webAuthn.rpDisplayName, rpDisplayName)
	}
}

func TestWebAuthn_BeginRegistration(t *testing.T) {
	rpID := "example.com"
	rpOrigin := "https://example.com"
	rpDisplayName := "Example"

	webAuthn, err := NewWebAuthn(rpID, rpOrigin, rpDisplayName)
	if err != nil {
		t.Fatalf("NewWebAuthn() error = %v", err)
	}

	userID := "test-user"
	cc, sessionData, err := webAuthn.BeginRegistration(userID)
	if err != nil {
		t.Fatalf("BeginRegistration() error = %v", err)
	}

	if cc.Response.Challenge == nil {
		t.Error("BeginRegistration() Challenge is nil")
	}
	if sessionData == nil {
		t.Error("BeginRegistration() sessionData is nil")
	}
}

func TestWebAuthn_FinishRegistration(t *testing.T) {
	rpID := "example.com"
	rpOrigin := "https://example.com"
	rpDisplayName := "Example"

	webAuthn, err := NewWebAuthn(rpID, rpOrigin, rpDisplayName)
	if err != nil {
		t.Fatalf("NewWebAuthn() error = %v", err)
	}

	userID := "test-user"
	cc, sessionData, err := webAuthn.BeginRegistration(userID)
	if err != nil {
		t.Fatalf("BeginRegistration() error = %v", err)
	}

	// Simulate a credential creation response
	cred := webauthn.Credential{
		ID:              []byte("test-credential-id"),
		PublicKey:       []byte("test-public-key"),
		AttestationType: "none",
		Authenticator: webauthn.Authenticator{
			AAGUID:       []byte("test-aaguid"),
			SignCount:    0,
			CloneWarning: false,
		},
	}

	err = webAuthn.FinishRegistration(userID, cred, sessionData)
	if err != nil {
		t.Fatalf("FinishRegistration() error = %v", err)
	}
}
