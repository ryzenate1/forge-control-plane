package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestManagerPreStartPatchesViaDisk(t *testing.T) {
	m := NewServerManager(nil)
	tmp := t.TempDir()
	serverID := "123e4567-e89b-12d3-a456-426614174000"
	// Setup state
	state := m.State(serverID)
	state.RootDir = tmp
	state.EnvVars = map[string]string{"SERVER_MEMORY": "1024"}
	state.AllocationPort = 25577
	state.AllocationIP = "0.0.0.0"
	state.ConfigurationSynced = true
	state.PowerState = PowerStateOffline

	// Write .config/server.json as beacon sync would
	payload := map[string]any{
		"config": map[string]any{
			"files": map[string]any{
				"server.properties": map[string]any{
					"parser": "properties",
					"find": map[string]any{
						"server-port": "{{server.build.default.port}}",
					},
				},
			},
		},
		"environment": map[string]any{"SERVER_MEMORY": "1024"},
		"allocations": map[string]any{"default": map[string]any{"port": 25577}},
	}
	dir := filepath.Join(tmp, ".config")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(payload)
	if err := os.WriteFile(filepath.Join(dir, "server.json"), b, 0640); err != nil {
		t.Fatal(err)
	}
	// Create initial server.properties with wrong port
	if err := os.WriteFile(filepath.Join(tmp, "server.properties"), []byte("server-port=25565\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Call pre-start patch directly
	if err := m.applyPreStartConfigPatches(serverID, tmp); err != nil {
		t.Fatalf("apply pre-start: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(tmp, "server.properties"))
	if !containsStr(string(data), "server-port=25577") {
		t.Errorf("pre-start patch failed, got %q", string(data))
	}
}

func TestManagerPreStartNoConfigNoOp(t *testing.T) {
	m := NewServerManager(nil)
	tmp := t.TempDir()
	serverID := "123e4567-e89b-12d3-a456-426614174001"
	state := m.State(serverID)
	state.RootDir = tmp
	state.ConfigurationSynced = true
	// No .config/server.json
	if err := m.applyPreStartConfigPatches(serverID, tmp); err != nil {
		t.Errorf("should not error when no config: %v", err)
	}
}
