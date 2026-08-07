//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// TestAuthenticateTimingEqualized verifies that unknown-email logins and
// known-email-with-wrong-password logins both fail and consume comparable
// bcrypt work, so response latency cannot be used to enumerate accounts
// (AUTH-002).
func TestAuthenticateTimingEqualized(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	email := "timing-" + uuid.NewString() + "@example.com"
	password := "CorrectHorse-BatteryStaple-1!"
	if _, err := s.CreateUser(ctx, CreateUserRequest{
		Email:    email,
		Password: password,
		Role:     "user",
	}, nil); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if _, err := s.Authenticate(ctx, email, password); err != nil {
		t.Fatalf("valid credentials rejected: %v", err)
	}

	start := time.Now()
	if _, err := s.Authenticate(ctx, email, "definitely-wrong-password"); err == nil {
		t.Fatal("wrong-password login succeeded")
	}
	wrongDur := time.Since(start)

	start = time.Now()
	if _, err := s.Authenticate(ctx, "nobody-"+uuid.NewString()+"@example.com", password); err == nil {
		t.Fatal("unknown-email login succeeded")
	}
	unknownDur := time.Since(start)

	// Both paths must have performed real bcrypt work. At the default cost a
	// hash comparison takes tens of milliseconds; if either path returns in
	// under 1ms the equalizer is not running.
	if wrongDur < time.Millisecond {
		t.Fatalf("wrong-password path too fast (%v): bcrypt skipped", wrongDur)
	}
	if unknownDur < time.Millisecond {
		t.Fatalf("unknown-email path too fast (%v): timing equalizer not applied", unknownDur)
	}
}

// TestDummyBcryptHashIsUsable guards the package-level timing-equalizer hash:
// it must be a well-formed bcrypt digest so CompareHashAndPassword performs
// full key-derivation work instead of failing fast on parse errors.
func TestDummyBcryptHashIsUsable(t *testing.T) {
	err := bcrypt.CompareHashAndPassword(dummyBcryptHash, []byte("forge-timing-equalizer"))
	if err == nil {
		t.Fatal("dummy hash must not verify against its placeholder plaintext")
	}
	if err != bcrypt.ErrMismatchedHashAndPassword {
		t.Fatalf("dummy hash malformed (expected mismatch, got %v)", err)
	}
}
