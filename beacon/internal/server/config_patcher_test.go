package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchPropertiesMapShapeMinecraft(t *testing.T) {
	tmp := t.TempDir()
	payload := map[string]any{
		"config": map[string]any{
			"files": map[string]any{
				"server.properties": map[string]any{
					"parser": "properties",
					"find": map[string]any{
						"server-ip":   "0.0.0.0",
						"server-port": "{{server.build.default.port}}",
						"query.port":  "{{server.build.default.port}}",
					},
				},
			},
		},
	}
	env := map[string]string{"SERVER_MEMORY": "1024"}
	port := 25577
	ip := "0.0.0.0"
	// create initial file with wrong port
	initial := "#Minecraft server properties\nserver-ip=127.0.0.1\nserver-port=25565\nmotd=test\n"
	if err := os.WriteFile(filepath.Join(tmp, "server.properties"), []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}
	if err := patchConfigurationFiles(tmp, payload, env, port, ip); err != nil {
		t.Fatalf("patch failed: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(tmp, "server.properties"))
	got := string(data)
	if !containsStr(got, "server-ip=0.0.0.0") {
		t.Errorf("expected server-ip patched, got %q", got)
	}
	if !containsStr(got, "server-port=25577") {
		t.Errorf("expected server-port patched to 25577, got %q", got)
	}
	if !containsStr(got, "query.port=25577") {
		t.Errorf("expected query.port patched, got %q", got)
	}
	if !containsStr(got, "motd=test") {
		t.Errorf("expected motd preserved, got %q", got)
	}
}

func TestPatchArrayShape(t *testing.T) {
	tmp := t.TempDir()
	payload := map[string]any{
		"config": map[string]any{
			"files": []any{
				map[string]any{
					"file":   "server.properties",
					"parser": "properties",
					"find": map[string]any{
						"server-port": "{{server.build.default.port}}",
					},
				},
			},
		},
	}
	env := map[string]string{}
	port := 25588
	if err := os.WriteFile(filepath.Join(tmp, "server.properties"), []byte("server-port=25565\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := patchConfigurationFiles(tmp, payload, env, port, "0.0.0.0"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(tmp, "server.properties"))
	if !containsStr(string(data), "server-port=25588") {
		t.Errorf("array shape not patched: %q", string(data))
	}
}

func TestPatchYamlAndJson(t *testing.T) {
	tmp := t.TempDir()
	// yaml
	payloadYaml := map[string]any{
		"config": map[string]any{
			"files": map[string]any{
				"config.yaml": map[string]any{
					"parser": "yaml",
					"find": map[string]any{
						"server.port": "{{server.build.default.port}}",
						"app.name":    "{{SERVER_NAME}}",
					},
				},
			},
		},
	}
	env := map[string]string{"SERVER_NAME": "MyServer"}
	if err := os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte("server:\n  port: 25565\napp:\n  name: old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := patchConfigurationFiles(tmp, payloadYaml, env, 25599, ""); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(tmp, "config.yaml"))
	if !containsStr(string(data), "port: 25599") {
		t.Errorf("yaml not patched: %q", string(data))
	}
	if !containsStr(string(data), "name: MyServer") {
		t.Errorf("yaml env not patched: %q", string(data))
	}
	// json
	payloadJson := map[string]any{
		"config": map[string]any{
			"files": map[string]any{
				"config.json": map[string]any{
					"parser": "json",
					"find": map[string]any{
						"server.port": "{{server.build.default.port}}",
					},
				},
			},
		},
	}
	if err := os.WriteFile(filepath.Join(tmp, "config.json"), []byte(`{"server":{"port":25565}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := patchConfigurationFiles(tmp, payloadJson, env, 25600, ""); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(tmp, "config.json"))
	if !containsStr(string(data), "25600") {
		t.Errorf("json not patched: %q", string(data))
	}
}

func TestPatchCreatesIfMissing(t *testing.T) {
	tmp := t.TempDir()
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
	}
	if err := patchConfigurationFiles(tmp, payload, map[string]string{}, 25565, ""); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(tmp, "server.properties"))
	if !containsStr(string(data), "server-port=25565") {
		t.Errorf("create missing file failed: %q", string(data))
	}
}

func TestPatchUnsupportedLogsWarn(t *testing.T) {
	tmp := t.TempDir()
	payload := map[string]any{
		"config": map[string]any{
			"files": map[string]any{
				"server.properties": map[string]any{
					"parser": "xml",
					"find": map[string]any{
						"server-port": "25565",
					},
				},
			},
		},
	}
	// Should not error, just log warn and skip
	if err := patchConfigurationFiles(tmp, payload, map[string]string{}, 25565, ""); err != nil {
		t.Fatal(err)
	}
	// File should not be created
	if _, err := os.Stat(filepath.Join(tmp, "server.properties")); !os.IsNotExist(err) {
		t.Errorf("unsupported parser should not create file")
	}
}

func containsStr(s, sub string) bool { return strings.Contains(s, sub) }
