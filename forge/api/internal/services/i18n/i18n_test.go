package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTranslationService(t *testing.T) {
	dir := t.TempDir()

	enContent := `{"greeting": "Hello", "nested": {"key": "Nested Value"}}`
	esContent := `{"greeting": "Hola", "nested": {"key": "Valor Anidado"}}`

	os.WriteFile(filepath.Join(dir, "en.json"), []byte(enContent), 0644)
	os.WriteFile(filepath.Join(dir, "es.json"), []byte(esContent), 0644)

	svc, err := New(Config{LangsDir: dir, Fallback: "en"})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		locale   string
		key      string
		expected string
	}{
		{"English greeting", "en", "greeting", "Hello"},
		{"Spanish greeting", "es", "greeting", "Hola"},
		{"English nested", "en", "nested.key", "Nested Value"},
		{"Spanish nested", "es", "nested.key", "Valor Anidado"},
		{"Fallback to English", "fr", "greeting", "Hello"},
		{"Missing key", "en", "nonexistent", "nonexistent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.T(tt.locale, tt.key)
			if got != tt.expected {
				t.Errorf("T(%q, %q) = %q, want %q", tt.locale, tt.key, got, tt.expected)
			}
		})
	}
}

func TestAvailableLocales(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"key": "val"}`), 0644)
	os.WriteFile(filepath.Join(dir, "de.json"), []byte(`{"key": "val"}`), 0644)

	svc, err := New(Config{LangsDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	locales := svc.AvailableLocales()
	if len(locales) != 2 {
		t.Errorf("expected 2 locales, got %d", len(locales))
	}
}
