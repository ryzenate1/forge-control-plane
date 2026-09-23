package events

import (
	"encoding/json"
	"strings"
	"testing"
)

// The Envelope.TenantID field and NewEnvelopeWithTenant constructor were
// removed: tenant/org hints now travel inside Payload only and are not
// promoted to envelope metadata. These tests pin that post-refactor shape.

func TestEnvelope_TenantHintStaysInPayload(t *testing.T) {
	env := NewEnvelope(EventServerCreated, "test", "server", "srv-1", map[string]any{"tenant_id": "org-123"})
	if env.Payload["tenant_id"] != "org-123" {
		t.Fatalf("expected tenant_id preserved in payload, got %v", env.Payload)
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// The tenant keys may appear inside the payload object, but the envelope
	// must not expose a top-level tenant field; decode and check.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["tenantId"]; ok {
		t.Fatalf("envelope must not serialize a top-level tenantId, got %s", data)
	}
	if _, ok := m["tenant_id"]; ok {
		t.Fatalf("envelope must not serialize a top-level tenant_id, got %s", data)
	}
}

func TestEnvelope_JSONRoundTrip(t *testing.T) {
	env := NewEnvelope(EventServerCreated, "test", "server", "srv-1", map[string]any{"foo": "bar"})
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"tenantId"`) {
		t.Fatalf("unexpected tenantId in envelope JSON: %s", data)
	}
	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ID != env.ID || decoded.Type != env.Type || decoded.ResourceID != "srv-1" {
		t.Fatalf("round trip mismatch: %+v", decoded)
	}
	if decoded.Payload["foo"] != "bar" {
		t.Fatalf("payload lost: %+v", decoded.Payload)
	}
}

func TestEnvelope_Validate(t *testing.T) {
	env := NewEnvelope(EventServerCreated, "test", "server", "srv-1", nil)
	if err := env.Validate(); err != nil {
		t.Fatalf("validate should pass: %v", err)
	}
	if env.Payload == nil {
		t.Fatal("nil payload should be normalized to empty map")
	}
}
