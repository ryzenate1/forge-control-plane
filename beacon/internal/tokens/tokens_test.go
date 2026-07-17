package tokens

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestGenerateAndValidateRoundTrip(t *testing.T) {
	g := NewGenerator([]byte("test-secret"))
	claims := Claims{
		Scope:     ScopeFileDownload,
		ServerID:  "srv-1",
		User:      "user-1",
		FilePath:  "/tmp/test.txt",
		UniqueID:  "uid-1",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	token, err := g.Generate(claims)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	got, err := g.Validate(token)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if got.Scope != claims.Scope {
		t.Fatalf("expected scope %q, got %q", claims.Scope, got.Scope)
	}
	if got.ServerID != claims.ServerID {
		t.Fatalf("expected server %q, got %q", claims.ServerID, got.ServerID)
	}
	if got.User != claims.User {
		t.Fatalf("expected user %q, got %q", claims.User, got.User)
	}
	if got.FilePath != claims.FilePath {
		t.Fatalf("expected filepath %q, got %q", claims.FilePath, got.FilePath)
	}
	if got.UniqueID != claims.UniqueID {
		t.Fatalf("expected unique_id %q, got %q", claims.UniqueID, got.UniqueID)
	}
}

func TestValidateRejectsExpired(t *testing.T) {
	g := NewGenerator([]byte("test-secret"))
	claims := Claims{
		Scope:     ScopeWebsocket,
		ServerID:  "srv-1",
		IssuedAt:  time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	token, err := g.Generate(claims)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	_, err = g.Validate(token)
	if err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestValidateRejectsTampered(t *testing.T) {
	g := NewGenerator([]byte("test-secret"))
	g2 := NewGenerator([]byte("wrong-secret"))

	claims := Claims{
		Scope:     ScopeWebsocket,
		ServerID:  "srv-1",
		ExpiresAt: time.Now().Add(time.Hour),
	}

	token, err := g.Generate(claims)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	_, err = g2.Validate(token)
	if err != ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestValidateRejectsMalformed(t *testing.T) {
	g := NewGenerator([]byte("test-secret"))

	_, err := g.Validate("only.two")
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for 2 parts, got %v", err)
	}

	_, err = g.Validate("one")
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for 1 part, got %v", err)
	}

	_, err = g.Validate("a.b.c.d")
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for 4 parts, got %v", err)
	}
}

func TestValidateRejectsInvalidBase64(t *testing.T) {
	g := NewGenerator([]byte("test-secret"))
	_, err := g.Validate("!!!.!!!.!!!")
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for bad base64, got %v", err)
	}
}

func TestValidateRejectsTamperedPayload(t *testing.T) {
	g := NewGenerator([]byte("test-secret"))
	claims := Claims{
		Scope:     ScopeWebsocket,
		ServerID:  "srv-1",
		ExpiresAt: time.Now().Add(time.Hour),
	}

	token, err := g.Generate(claims)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	parts := strings.Split(token, ".")
	fakePayload := base64.RawURLEncoding.EncodeToString([]byte(`{"scope":"websocket","server_id":"hacked","exp":"2099-01-01T00:00:00Z"}`))
	tampered := parts[0] + "." + fakePayload + "." + parts[2]

	_, err = g.Validate(tampered)
	if err != ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature for tampered payload, got %v", err)
	}
}

func TestGenerateFileDownload(t *testing.T) {
	g := NewGenerator([]byte("test-secret"))
	token, err := g.GenerateFileDownload("srv-1", "/data/world.dat", "user-1", time.Hour)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	claims, err := g.Validate(token)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if claims.Scope != ScopeFileDownload {
		t.Fatalf("expected scope %q, got %q", ScopeFileDownload, claims.Scope)
	}
	if claims.ServerID != "srv-1" {
		t.Fatalf("expected server srv-1, got %q", claims.ServerID)
	}
	if claims.FilePath != "/data/world.dat" {
		t.Fatalf("expected file path /data/world.dat, got %q", claims.FilePath)
	}
	if claims.User != "user-1" {
		t.Fatalf("expected user user-1, got %q", claims.User)
	}
	if claims.UniqueID == "" {
		t.Fatal("expected non-empty unique_id")
	}
}

func TestGenerateBackupDownload(t *testing.T) {
	g := NewGenerator([]byte("test-secret"))
	token, err := g.GenerateBackupDownload("srv-1", "bk-123", "user-1", time.Hour)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	claims, err := g.Validate(token)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if claims.Scope != ScopeBackupDownload {
		t.Fatalf("expected scope %q, got %q", ScopeBackupDownload, claims.Scope)
	}
	if claims.BackupID != "bk-123" {
		t.Fatalf("expected backup_id bk-123, got %q", claims.BackupID)
	}
	if claims.UniqueID == "" {
		t.Fatal("expected non-empty unique_id")
	}
}

func TestGenerateUpload(t *testing.T) {
	g := NewGenerator([]byte("test-secret"))
	token, err := g.GenerateUpload("srv-1", "user-1", time.Hour)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	claims, err := g.Validate(token)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if claims.Scope != ScopeFileUpload {
		t.Fatalf("expected scope %q, got %q", ScopeFileUpload, claims.Scope)
	}
	if claims.ServerID != "srv-1" {
		t.Fatalf("expected server srv-1, got %q", claims.ServerID)
	}
	if claims.UniqueID == "" {
		t.Fatal("expected non-empty unique_id")
	}
}

func TestGenerateWebsocket(t *testing.T) {
	g := NewGenerator([]byte("test-secret"))
	token, err := g.GenerateWebsocket("srv-1", "user-1", time.Hour)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	claims, err := g.Validate(token)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if claims.Scope != ScopeWebsocket {
		t.Fatalf("expected scope %q, got %q", ScopeWebsocket, claims.Scope)
	}
	if claims.ServerID != "srv-1" {
		t.Fatalf("expected server srv-1, got %q", claims.ServerID)
	}
	if claims.User != "user-1" {
		t.Fatalf("expected user user-1, got %q", claims.User)
	}
}

func TestTokenStoreAddAndIsValid(t *testing.T) {
	ts := NewTokenStore()
	ts.Add("uid-1", time.Now().Add(time.Hour))

	if !ts.IsValid("uid-1") {
		t.Fatal("first IsValid call should return true")
	}
	if ts.IsValid("uid-1") {
		t.Fatal("second IsValid call should return false (one-time use)")
	}
}

func TestTokenStoreIsValidUnknown(t *testing.T) {
	ts := NewTokenStore()
	if ts.IsValid("nonexistent") {
		t.Fatal("IsValid should return false for unknown ID")
	}
}

func TestTokenStoreIsValidExpired(t *testing.T) {
	ts := NewTokenStore()
	ts.Add("uid-expired", time.Now().Add(-time.Hour))
	if ts.IsValid("uid-expired") {
		t.Fatal("IsValid should return false for expired token")
	}
}

func TestTokenStoreCleanup(t *testing.T) {
	ts := NewTokenStore()
	ts.Add("uid-valid", time.Now().Add(time.Hour))
	ts.Add("uid-expired1", time.Now().Add(-time.Hour))
	ts.Add("uid-expired2", time.Now().Add(-time.Minute))

	ts.Cleanup()

	ts.mu.Lock()
	_, validExists := ts.tokens["uid-valid"]
	_, exp1Exists := ts.tokens["uid-expired1"]
	_, exp2Exists := ts.tokens["uid-expired2"]
	ts.mu.Unlock()

	if !validExists {
		t.Fatal("valid token should still exist after cleanup")
	}
	if exp1Exists {
		t.Fatal("expired token 1 should be removed after cleanup")
	}
	if exp2Exists {
		t.Fatal("expired token 2 should be removed after cleanup")
	}
}

func TestWebSocketDenylistDenyAndCheck(t *testing.T) {
	d := NewWebSocketDenylist()
	d.DenyForServer("srv-1", "user-1")

	if !d.IsDenied("srv-1", "user-1") {
		t.Fatal("user-1 should be denied for srv-1")
	}
	if d.IsDenied("srv-1", "user-2") {
		t.Fatal("user-2 should not be denied for srv-1")
	}
	if d.IsDenied("srv-2", "user-1") {
		t.Fatal("user-1 should not be denied for srv-2")
	}
}

func TestWebSocketDenylistIsDeniedUnknown(t *testing.T) {
	d := NewWebSocketDenylist()
	if d.IsDenied("unknown-server", "unknown-user") {
		t.Fatal("should return false for unknown server/user")
	}
}

func TestIsBeforeBoot(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	if !IsBeforeBoot(past) {
		t.Fatal("past timestamp should be before boot")
	}

	future := time.Now().Add(1 * time.Hour)
	if IsBeforeBoot(future) {
		t.Fatal("future timestamp should not be before boot")
	}
}
