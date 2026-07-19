package http

import (
	"testing"
	"time"
)

func TestWSTicketSignAndVerify(t *testing.T) {
	cfg := Config{AuthSecret: "test-secret-12345678"}
	store := newWSTicketStore(cfg)
	put := store.put
	_ = put

	// Manually create a ticket and put it in the store.
	store.put(wsTicket{
		Subject:   "abcd1234",
		ServerID:  "server-1",
		Stream:    "console",
		ExpiresAt: time.Now().Add(60 * time.Second),
	})

	token := signTicket(cfg.AuthSecret, "abcd1234")
	if token == "" {
		t.Fatal("expected token")
	}
	serverID, stream, ok := VerifyWSTicket(cfg, store, token)
	if !ok {
		t.Fatal("expected ticket to verify")
	}
	if serverID != "server-1" || stream != "console" {
		t.Fatalf("unexpected server/stream: %q / %q", serverID, stream)
	}

	// Second use should fail (consumed).
	_, _, ok = VerifyWSTicket(cfg, store, token)
	if ok {
		t.Fatal("ticket should be single-use")
	}

	// Tampered token.
	tampered := token + "garbage"
	_, _, ok = VerifyWSTicket(cfg, store, tampered)
	if ok {
		t.Fatal("tampered token should not verify")
	}

	// Wrong secret.
	bad := signTicket("wrong-secret", "abcd1234")
	// The token is gone from the store, so re-add and try.
	store.put(wsTicket{Subject: "abcd1234", ServerID: "server-1", Stream: "console", ExpiresAt: time.Now().Add(60 * time.Second)})
	_, _, ok = VerifyWSTicket(cfg, store, bad)
	if ok {
		t.Fatal("wrong-secret signature should not verify")
	}
}

func TestWSTicketInspectionDoesNotConsume(t *testing.T) {
	cfg := Config{AuthSecret: "test-secret"}
	store := newWSTicketStore(cfg)
	store.put(wsTicket{
		Subject:   "inspectable",
		UserID:    "user-1",
		ServerID:  "server-1",
		Stream:    "console",
		ExpiresAt: time.Now().Add(time.Minute),
	})
	token := signTicket(cfg.AuthSecret, "inspectable")
	ticket, ok := inspectWSTicket(cfg, store, token)
	if !ok || ticket.UserID != "user-1" {
		t.Fatalf("inspection returned %#v, %v", ticket, ok)
	}
	if _, ok := inspectWSTicket(cfg, store, token); !ok {
		t.Fatal("inspection must not consume the ticket")
	}
	if !consumeWSTicket(cfg, store, token) {
		t.Fatal("expected ticket consumption to succeed")
	}
	if _, ok := inspectWSTicket(cfg, store, token); ok {
		t.Fatal("consumed ticket must not be inspectable")
	}
}

func TestWSTicketExpired(t *testing.T) {
	cfg := Config{AuthSecret: "test-secret"}
	store := newWSTicketStore(cfg)
	store.put(wsTicket{
		Subject:   "expired",
		ServerID:  "s",
		Stream:    "console",
		ExpiresAt: time.Now().Add(-1 * time.Second),
	})
	token := signTicket(cfg.AuthSecret, "expired")
	_, _, ok := VerifyWSTicket(cfg, store, token)
	if ok {
		t.Fatal("expired ticket should not verify")
	}
}
