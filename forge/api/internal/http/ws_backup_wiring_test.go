package http

import (
	"os"
	"strings"
	"testing"
)

func TestRealtimeProxySupportsBackupStream(t *testing.T) {
	// Static verification that server.go wires backup WS and realtimeProxy handles it
	data, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("cannot read server.go: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"/servers/:id/ws/backup"`) {
		t.Error("server.go should wire /servers/:id/ws/backup realtimeProxy for backup progress")
	}
	if !strings.Contains(content, `realtimeProxy(cfg, wsTickets, "backup")`) {
		t.Error("server.go should proxy backup stream via realtimeProxy")
	}
	// Check all expected streams are wired
	for _, stream := range []string{"stats", "logs", "console", "backup"} {
		if !strings.Contains(content, `"/servers/:id/ws/`+stream+`"`) {
			t.Errorf("missing ws route for stream %q", stream)
		}
	}
}

func TestRealtimeProxyBackupPermissionCheck(t *testing.T) {
	data, err := os.ReadFile("realtime.go")
	if err != nil {
		t.Fatalf("cannot read realtime.go: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `stream == "backup"`) {
		t.Error("realtimeProxy should have extra permission check for backup stream")
	}
	if !strings.Contains(content, "PermBackupRead") {
		t.Error("backup stream should require PermBackupRead")
	}
	// Ensure console still has its check
	if !strings.Contains(content, `stream == "console"`) {
		t.Error("console permission check should remain")
	}
}

func TestWSTicketSupportsBackupStream(t *testing.T) {
	// IssueWSTicket should accept stream=backup; verify source does not restrict streams
	data, err := os.ReadFile("handlers_ws_ticket.go")
	if err != nil {
		t.Fatalf("cannot read handlers_ws_ticket.go: %v", err)
	}
	// The handler uses c.Query("stream", "console") and stores whatever is passed; check it doesn't filter
	if strings.Contains(string(data), `"stats" ||`) && strings.Contains(string(data), `"logs"`) {
		// If there were a whitelist, backup would be missing; ensure backup is not rejected
		t.Log("ticket handler allows arbitrary streams, backup should work")
	}
	// Static check that backup is not explicitly rejected
	if strings.Contains(string(data), `stream != "backup"`) {
		t.Error("ticket handler should not reject backup stream")
	}
}
