package auth

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestCookieSessionStore_Create(t *testing.T) {
	store := NewCookieSessionStore("session", true, true, http.SameSiteLaxMode)
	session := Session{
		ID:        "test-session",
		UserID:    "test-user",
		Scopes:    Scopes{ScopeServerRead, ScopeServerWrite},
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	err := store.Create(context.Background(), session)
	if err != nil {
		t.Errorf("Create() error = %v", err)
	}
}

func TestCookieSessionStore_Get(t *testing.T) {
	store := NewCookieSessionStore("session", true, true, http.SameSiteLaxMode)
	session := Session{
		ID:        "test-session",
		UserID:    "test-user",
		Scopes:    Scopes{ScopeServerRead, ScopeServerWrite},
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	// First create the session
	err := store.Create(context.Background(), session)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Then retrieve it
	retrieved, err := store.Get(context.Background(), "test-session")
	if err != nil {
		t.Errorf("Get() error = %v", err)
	}

	if retrieved.ID != session.ID {
		t.Errorf("Get() ID = %v, want %v", retrieved.ID, session.ID)
	}
	if retrieved.UserID != session.UserID {
		t.Errorf("Get() UserID = %v, want %v", retrieved.UserID, session.UserID)
	}
}

func TestCookieSessionStore_Delete(t *testing.T) {
	store := NewCookieSessionStore("session", true, true, http.SameSiteLaxMode)
	session := Session{
		ID:        "test-session",
		UserID:    "test-user",
		Scopes:    Scopes{ScopeServerRead, ScopeServerWrite},
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	// First create the session
	err := store.Create(context.Background(), session)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Then delete it
	err = store.Delete(context.Background(), "test-session")
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify it's gone
	_, err = store.Get(context.Background(), "test-session")
	if err == nil {
		t.Error("Get() did not return error for deleted session")
	}
}
