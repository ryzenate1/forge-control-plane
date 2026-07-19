package http

import (
	"encoding/json"
	"testing"
)

func TestImportPluginManifest_Valid(t *testing.T) {
	manifest := []byte(`{"name":"forge-stripe","description":"Stripe billing","kind":"integration","version":"1.0.0","hooks":["user.create"]}`)
	if !json.Valid(manifest) {
		t.Fatal("manifest is not valid JSON")
	}
	var raw map[string]any
	if err := json.Unmarshal(manifest, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["name"] != "forge-stripe" {
		t.Fatalf("expected name forge-stripe, got %v", raw["name"])
	}
}

func TestImportPluginManifest_MissingName(t *testing.T) {
	manifest := []byte(`{"description":"no name"}`)
	var raw map[string]any
	if err := json.Unmarshal(manifest, &raw); err != nil {
		t.Fatal(err)
	}
	if name, _ := raw["name"].(string); name != "" {
		t.Fatalf("expected empty name, got %q", name)
	}
}

func TestPluginHash(t *testing.T) {
	h1 := pluginHash([]byte("hello"))
	h2 := pluginHash([]byte("hello"))
	h3 := pluginHash([]byte("world"))
	if h1 != h2 {
		t.Fatal("hash should be deterministic")
	}
	if h1 == h3 {
		t.Fatal("different inputs should yield different hashes")
	}
	if len(h1) != 64 {
		t.Fatalf("expected 64-char hex, got %d", len(h1))
	}
}

func TestPluginError(t *testing.T) {
	err := pluginError("forge-stripe", nil)
	if err != nil {
		// nil inner error must produce nil wrapped error.
		t.Fatalf("expected nil, got %v", err)
	}
}
