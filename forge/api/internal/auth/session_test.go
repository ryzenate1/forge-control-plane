package auth

import (
	"context"
	"testing"
	"time"
)

func TestGenerateSessionToken(t *testing.T) {
	token, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}

	token2, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == token2 {
		t.Error("expected unique tokens")
	}
}

func TestGenerateSessionID(t *testing.T) {
	id, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty id")
	}
}

func TestInMemorySessionStoreCreate(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	sess := &Session{
		ID:           "sess-1",
		UserID:       "user-1",
		Token:        "token-1",
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(time.Hour),
		LastActiveAt: time.Now(),
		IPAddress:    "127.0.0.1",
		UserAgent:    "test-agent",
	}

	err := store.Create(ctx, sess)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInMemorySessionStoreCreateNil(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	err := store.Create(ctx, nil)
	if err == nil {
		t.Error("expected error for nil session")
	}
}

func TestInMemorySessionStoreCreateMissingFields(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	err := store.Create(ctx, &Session{ID: "x"})
	if err == nil {
		t.Error("expected error for missing required fields")
	}
}

func TestInMemorySessionStoreGet(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	sess := &Session{
		ID:        "sess-1",
		UserID:    "user-1",
		Token:     "token-1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	_ = store.Create(ctx, sess)

	got, err := store.Get(ctx, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "sess-1" {
		t.Errorf("expected ID 'sess-1', got %q", got.ID)
	}
}

func TestInMemorySessionStoreGetNotFound(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestInMemorySessionStoreGetByToken(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	sess := &Session{
		ID:        "sess-1",
		UserID:    "user-1",
		Token:     "token-1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	_ = store.Create(ctx, sess)

	got, err := store.GetByToken(ctx, "token-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.UserID != "user-1" {
		t.Errorf("expected UserID 'user-1', got %q", got.UserID)
	}
}

func TestInMemorySessionStoreGetByTokenNotFound(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	_, err := store.GetByToken(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent token")
	}
}

func TestInMemorySessionStoreUpdate(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	sess := &Session{
		ID:        "sess-1",
		UserID:    "user-1",
		Token:     "token-1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
		IPAddress: "127.0.0.1",
	}
	_ = store.Create(ctx, sess)

	sess.IPAddress = "192.168.1.1"
	err := store.Update(ctx, sess)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := store.Get(ctx, "sess-1")
	if got.IPAddress != "192.168.1.1" {
		t.Errorf("expected updated IP, got %q", got.IPAddress)
	}
}

func TestInMemorySessionStoreUpdateNotFound(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	err := store.Update(ctx, &Session{ID: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestInMemorySessionStoreDelete(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	sess := &Session{
		ID:        "sess-1",
		UserID:    "user-1",
		Token:     "token-1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	_ = store.Create(ctx, sess)

	err := store.Delete(ctx, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = store.Get(ctx, "sess-1")
	if err == nil {
		t.Error("expected error after deletion")
	}

	_, err = store.GetByToken(ctx, "token-1")
	if err == nil {
		t.Error("expected error after deletion (by token)")
	}
}

func TestInMemorySessionStoreDeleteNonexistent(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	err := store.Delete(ctx, "nonexistent")
	if err != nil {
		t.Errorf("expected no error for deleting nonexistent, got: %v", err)
	}
}

func TestInMemorySessionStoreDeleteByUser(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		sess := &Session{
			ID:        "sess-" + string(rune('a'+i)),
			UserID:    "user-1",
			Token:     "token-" + string(rune('a'+i)),
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
		}
		_ = store.Create(ctx, sess)
	}

	err := store.DeleteByUser(ctx, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sessions, _ := store.ListByUser(ctx, "user-1")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after DeleteByUser, got %d", len(sessions))
	}
}

func TestInMemorySessionStoreCleanup(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	active := &Session{
		ID:        "active",
		UserID:    "user-1",
		Token:     "token-active",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	expired := &Session{
		ID:        "expired",
		UserID:    "user-2",
		Token:     "token-expired",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	_ = store.Create(ctx, active)
	_ = store.Create(ctx, expired)

	err := store.Cleanup(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = store.Get(ctx, "active")
	if err != nil {
		t.Error("expected active session to survive cleanup")
	}

	_, err = store.Get(ctx, "expired")
	if err == nil {
		t.Error("expected expired session to be cleaned up")
	}
}

func TestInMemorySessionStoreListByUser(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	_ = store.Create(ctx, &Session{
		ID: "s1", UserID: "u1", Token: "t1",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	})
	_ = store.Create(ctx, &Session{
		ID: "s2", UserID: "u1", Token: "t2",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	})
	_ = store.Create(ctx, &Session{
		ID: "s3", UserID: "u2", Token: "t3",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	})

	sessions, err := store.ListByUser(ctx, "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions for u1, got %d", len(sessions))
	}
}

func TestSessionContext(t *testing.T) {
	sess := &Session{ID: "s1", UserID: "u1"}
	ctx := ContextWithSession(context.Background(), sess)

	got := SessionFromContext(ctx)
	if got == nil {
		t.Fatal("expected session from context")
	}
	if got.ID != "s1" {
		t.Errorf("expected ID 's1', got %q", got.ID)
	}
}

func TestSessionContextNil(t *testing.T) {
	got := SessionFromContext(context.Background())
	if got != nil {
		t.Error("expected nil session from empty context")
	}
}
