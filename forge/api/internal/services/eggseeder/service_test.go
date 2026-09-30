package eggseeder

import (
	"encoding/json"
	"testing"
)

func TestEmbeddedTemplatesCount(t *testing.T) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("read embedded templates: %v", err)
	}
	if len(entries) < 14 {
		t.Fatalf("expected at least 14 embedded templates, got %d", len(entries))
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := templateFS.ReadFile("templates/" + e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		for _, key := range []string{"id", "name", "install_script", "startup"} {
			if _, ok := m[key]; !ok {
				t.Fatalf("%s missing required field %q", e.Name(), key)
			}
		}
		if is, ok := m["install_script"].(map[string]interface{}); ok {
			for _, k := range []string{"container", "entrypoint", "script"} {
				if _, ok := is[k]; !ok {
					t.Fatalf("%s install_script missing %q", e.Name(), k)
				}
				if s, ok := is[k].(string); !ok || s == "" {
					t.Fatalf("%s install_script.%s must be non-empty string", e.Name(), k)
				}
			}
		} else {
			t.Fatalf("%s install_script not an object", e.Name())
		}
	}
}

func TestEmbeddedTemplatesInstallScriptPlaceholders(t *testing.T) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("read embedded templates: %v", err)
	}
	for _, e := range entries {
		data, _ := templateFS.ReadFile("templates/" + e.Name())
		var tmpl templateFile
		if err := json.Unmarshal(data, &tmpl); err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		if tmpl.InstallScript.Script == "" {
			t.Fatalf("%s install_script.script empty", e.Name())
		}
		if tmpl.InstallScript.Container == "" || tmpl.InstallScript.Entrypoint == "" {
			t.Fatalf("%s install_script container/entrypoint empty", e.Name())
		}
	}
}
