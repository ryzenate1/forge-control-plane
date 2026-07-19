package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type TranslationService struct {
	mu           sync.RWMutex
	translations map[string]map[string]string
	fallback     string
	langsDir     string
}

type Config struct {
	LangsDir string
	Fallback string
}

func New(cfg Config) (*TranslationService, error) {
	s := &TranslationService{
		translations: make(map[string]map[string]string),
		fallback:     cfg.Fallback,
		langsDir:     cfg.LangsDir,
	}
	if cfg.Fallback == "" {
		s.fallback = "en"
	}
	if err := s.loadAll(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *TranslationService) loadAll() error {
	entries, err := os.ReadDir(s.langsDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		locale := entry.Name()[:len(entry.Name())-5]
		data, err := os.ReadFile(filepath.Join(s.langsDir, entry.Name()))
		if err != nil {
			return err
		}
		var flat map[string]string
		if err := json.Unmarshal(data, &flat); err != nil {
			nested := make(map[string]any)
			if err2 := json.Unmarshal(data, &nested); err2 != nil {
				return err
			}
			flat = flatten(nested, "")
		}
		s.mu.Lock()
		s.translations[locale] = flat
		s.mu.Unlock()
	}
	return nil
}

func flatten(nested map[string]any, prefix string) map[string]string {
	result := make(map[string]string)
	for key, val := range nested {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}
		switch v := val.(type) {
		case string:
			result[fullKey] = v
		case map[string]any:
			for k, v2 := range flatten(v, fullKey) {
				result[k] = v2
			}
		}
	}
	return result
}

func (s *TranslationService) T(locale, key string, args ...any) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if langs, ok := s.translations[locale]; ok {
		if val, ok := langs[key]; ok {
			if len(args) > 0 {
				return fmt.Sprintf(val, args...)
			}
			return val
		}
	}

	if langs, ok := s.translations[s.fallback]; ok {
		if val, ok := langs[key]; ok {
			if len(args) > 0 {
				return fmt.Sprintf(val, args...)
			}
			return val
		}
	}

	return key
}

func (s *TranslationService) AvailableLocales() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	locales := make([]string, 0, len(s.translations))
	for locale := range s.translations {
		locales = append(locales, locale)
	}
	return locales
}

func (s *TranslationService) Reload() error {
	return s.loadAll()
}
