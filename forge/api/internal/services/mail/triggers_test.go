package mail

import (
	"context"
	"strings"
	"testing"
)

func TestSanitizeHeaderValue(t *testing.T) {
	if got := sanitizeHeaderValue("Server Created: test\r\nBcc: evil@x"); strings.ContainsAny(got, "\r\n") {
		t.Fatalf("newlines survived sanitization: %q", got)
	}
	if got := sanitizeHeaderValue("  hello   world  "); got != "hello world" {
		t.Fatalf("expected collapsed whitespace, got %q", got)
	}
	if got := sanitizeHeaderValue(strings.Repeat("a", 500)); len(got) > 200 {
		t.Fatalf("expected capped length, got %d", len(got))
	}
}

func TestWorkerEnqueueRejectsNewlineRecipient(t *testing.T) {
	w := &Worker{}
	err := w.Enqueue(context.Background(), "victim@x\r\nBcc: evil@x", "subject", "text", "html")
	if err == nil {
		t.Fatal("expected error for newline recipient, got nil")
	}
}
