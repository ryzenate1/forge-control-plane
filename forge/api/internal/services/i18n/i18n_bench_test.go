package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkTranslation(b *testing.B) {
	dir := b.TempDir()
	enContent := `{"greeting": "Hello", "nested": {"key": "Value"}, "common": {"save": "Save", "cancel": "Cancel", "delete": "Delete"}}`
	os.WriteFile(filepath.Join(dir, "en.json"), []byte(enContent), 0644)
	os.WriteFile(filepath.Join(dir, "es.json"), []byte(`{"greeting": "Hola", "nested": {"key": "Valor"}, "common": {"save": "Guardar", "cancel": "Cancelar", "delete": "Eliminar"}}`), 0644)

	svc, err := New(Config{LangsDir: dir, Fallback: "en"})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			svc.T("en", "greeting")
			svc.T("es", "common.save")
			svc.T("fr", "nested.key") // fallback
		}
	})
}

func BenchmarkTranslationParallel(b *testing.B) {
	dir := b.TempDir()
	os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"key": "value"}`), 0644)
	svc, _ := New(Config{LangsDir: dir})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			svc.T("en", "key")
		}
	})
}
