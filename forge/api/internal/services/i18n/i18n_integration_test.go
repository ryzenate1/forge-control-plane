package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIntegration_LoadRealTranslations(t *testing.T) {
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run")
	}

	langDir := filepath.Join("..", "..", "..", "..", "..", "lang")
	if _, err := os.Stat(langDir); os.IsNotExist(err) {
		langDir = filepath.Join("..", "..", "..", "..", "lang")
	}

	svc, err := New(Config{LangsDir: langDir, Fallback: "en"})
	if err != nil {
		t.Fatalf("failed to load translations from %s: %v", langDir, err)
	}

	locales := svc.AvailableLocales()
	if len(locales) == 0 {
		t.Fatal("no locales loaded")
	}

	t.Logf("Loaded %d locales: %v", len(locales), locales)

	greeting := svc.T("en", "common.save")
	if greeting == "common.save" {
		t.Error("expected translation for common.save, got key back")
	}

	fallback := svc.T("zz", "common.save")
	if fallback == "common.save" {
		t.Error("expected fallback translation for common.save")
	}
}
