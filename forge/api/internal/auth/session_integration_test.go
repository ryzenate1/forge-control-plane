package auth

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSessionStoreCreateGetDelete(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	sess := &Session{
		ID:        "sess-integ-1",
		UserID:    "user-integ-1",
		Token:     "token-integ-1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}

	if err := store.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.Get(ctx, "sess-integ-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "sess-integ-1" {
		t.Errorf("expected ID sess-integ-1, got %s", got.ID)
	}

	if err := store.Delete(ctx, "sess-integ-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = store.Get(ctx, "sess-integ-1")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestSessionExpiryAndCleanup(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	active := &Session{
		ID: "active-sess", UserID: "user-1", Token: "token-active",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	expired := &Session{
		ID: "expired-sess", UserID: "user-2", Token: "token-expired",
		CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	expired2 := &Session{
		ID: "expired-sess-2", UserID: "user-2", Token: "token-expired-2",
		CreatedAt: time.Now().Add(-3 * time.Hour), ExpiresAt: time.Now().Add(-2 * time.Hour),
	}

	for _, s := range []*Session{active, expired, expired2} {
		if err := store.Create(ctx, s); err != nil {
			t.Fatalf("Create %s: %v", s.ID, err)
		}
	}

	if err := store.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	_, err := store.Get(ctx, "active-sess")
	if err != nil {
		t.Error("expected active session to survive cleanup")
	}

	_, err = store.Get(ctx, "expired-sess")
	if err == nil {
		t.Error("expected expired session to be cleaned up")
	}

	_, err = store.Get(ctx, "expired-sess-2")
	if err == nil {
		t.Error("expected second expired session to be cleaned up")
	}

	sessions, err := store.ListByUser(ctx, "user-2")
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for user-2 after cleanup, got %d", len(sessions))
	}
}

func TestConcurrentSessionAccess(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	var wg sync.WaitGroup
	n := 50

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sess := &Session{
				ID:        "sess-{idx}",
				UserID:    "user-{idx}",
				Token:     "token-{idx}",
				CreatedAt: time.Now(),
				ExpiresAt: time.Now().Add(time.Hour),
			}
			_ = store.Create(ctx, sess)
		}(i)
	}
	wg.Wait()

	activeSessions := 0
	for i := 0; i < n; i++ {
		if _, err := store.Get(ctx, "sess-{idx}"); err == nil {
			activeSessions++
		}
	}

	wg = sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sess := &Session{
				ID:        "sess-con-{idx}",
				UserID:    "user-concurrent",
				Token:     "token-con-{idx}",
				CreatedAt: time.Now(),
				ExpiresAt: time.Now().Add(time.Hour),
			}
			_ = store.Create(ctx, sess)
		}(i)
	}
	wg.Wait()

	sessions, err := store.ListByUser(ctx, "user-concurrent")
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(sessions) != n {
		t.Errorf("expected %d sessions for concurrent user, got %d", n, len(sessions))
	}
}

func TestConcurrentReadWriteNoDeadlock(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		sess := &Session{
			ID: "sess-dl-{i}", UserID: "user-dl", Token: "token-dl-{i}",
			CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
		}
		_ = store.Create(ctx, sess)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = store.Get(ctx, "sess-dl-0")
			_, _ = store.ListByUser(ctx, "user-dl")
			_ = store.Cleanup(ctx)
		}()
	}
	wg.Wait()
}

func TestSessionStoreAfterSimulatedRestart(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	sess := &Session{
		ID: "pre-restart", UserID: "user-restart", Token: "token-restart",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	store2 := NewInMemorySessionStore()
	sess2 := &Session{
		ID: "post-restart", UserID: "user-restart", Token: "token-restart-2",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store2.Create(ctx, sess2); err != nil {
		t.Fatalf("Create after restart: %v", err)
	}

	_, err := store2.Get(ctx, "pre-restart")
	if err == nil {
		t.Error("expected pre-restart session to not exist in new store")
	}

	got, err := store2.Get(ctx, "post-restart")
	if err != nil {
		t.Fatalf("Get post-restart session: %v", err)
	}
	if got.ID != "post-restart" {
		t.Errorf("expected post-restart session, got %s", got.ID)
	}
}

func TestSessionGetByTokenAfterDelete(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	sess := &Session{
		ID: "sess-by-token", UserID: "user-token", Token: "lookup-token",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.GetByToken(ctx, "lookup-token")
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got.ID != "sess-by-token" {
		t.Errorf("expected ID sess-by-token, got %s", got.ID)
	}

	if err := store.Delete(ctx, "sess-by-token"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = store.GetByToken(ctx, "lookup-token")
	if err == nil {
		t.Error("expected GetByToken to fail after delete")
	}
}

func TestHashToken(t *testing.T) {
	token := "my-secret-token-value"
	hash := hashToken(token)
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if len(hash) != 64 {
		t.Errorf("expected SHA256 hex length 64, got %d", len(hash))
	}

	hash2 := hashToken(token)
	if hash != hash2 {
		t.Error("expected same hash for same token")
	}

	hash3 := hashToken("different-token")
	if hash == hash3 {
		t.Error("expected different hash for different token")
	}
}

func TestSessionStoreMultipleUsers(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	users := []string{"alice", "bob", "charlie"}
	for _, user := range users {
		for i := 0; i < 3; i++ {
			sess := &Session{
				ID: user + "-sess-{i}", UserID: user,
				Token: user + "-token-{i}",
				CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
			}
			if err := store.Create(ctx, sess); err != nil {
				t.Fatalf("Create %s: %v", sess.ID, err)
			}
		}
	}

	for _, user := range users {
		sessions, err := store.ListByUser(ctx, user)
		if err != nil {
			t.Fatalf("ListByUser %s: %v", user, err)
		}
		if len(sessions) != 3 {
			t.Errorf("expected 3 sessions for %s, got %d", user, len(sessions))
		}
	}

	if err := store.DeleteByUser(ctx, "bob"); err != nil {
		t.Fatalf("DeleteByUser bob: %v", err)
	}

	bobSessions, _ := store.ListByUser(ctx, "bob")
	if len(bobSessions) != 0 {
		t.Errorf("expected 0 sessions for bob after DeleteByUser, got %d", len(bobSessions))
	}

	aliceSessions, _ := store.ListByUser(ctx, "alice")
	if len(aliceSessions) != 3 {
		t.Errorf("expected 3 sessions for alice (unaffected), got %d", len(aliceSessions))
	}
}
