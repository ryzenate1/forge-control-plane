package http

import (
	"testing"
	"time"
)

func TestFileDownloadTicketIsSingleUseAndBoundToPath(t *testing.T) {
	store := newFileDownloadTicketStore()
	token, err := store.issue(fileDownloadTicket{serverID: "server-a", filePath: "mods/game.bin", expires: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	ticket, ok := store.consume(token)
	if !ok || ticket.serverID != "server-a" || ticket.filePath != "mods/game.bin" {
		t.Fatalf("unexpected ticket: ok=%v ticket=%+v", ok, ticket)
	}
	if _, ok := store.consume(token); ok {
		t.Fatal("ticket was consumed more than once")
	}
}

func TestExpiredFileDownloadTicketIsRejected(t *testing.T) {
	store := newFileDownloadTicketStore()
	token, err := store.issue(fileDownloadTicket{serverID: "server-a", filePath: "game.bin", expires: time.Now().Add(-time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.consume(token); ok {
		t.Fatal("expired ticket was accepted")
	}
}
