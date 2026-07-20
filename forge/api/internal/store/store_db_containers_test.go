package store

import (
	"testing"
)

func TestValidateDBEngine(t *testing.T) {
	tests := []struct {
		engine  string
		version string
		wantErr bool
	}{
		{"postgresql", "15", false},
		{"postgresql", "16", false},
		{"postgresql", "13", false},
		{"postgresql", "99", true},
		{"mysql", "8.0", false},
		{"mysql", "8.3", false},
		{"mysql", "5.7", true},
		{"mariadb", "10", false},
		{"mariadb", "11", false},
		{"mariadb", "12", true},
		{"redis", "7", false},
		{"redis", "6", false},
		{"redis", "5", true},
		{"mongodb", "7", false},
		{"mongodb", "6", false},
		{"mongodb", "8", true},
		{"postgres", "15", false},
		{"", "15", true},
		{"unknown", "1", true},
		{"PostgreSQL", "15", false},
		{"POSTGRESQL", "16", false},
	}

	for _, tt := range tests {
		t.Run(tt.engine+"_"+tt.version, func(t *testing.T) {
			err := ValidateDBEngine(tt.engine, tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDBEngine(%q, %q) error = %v, wantErr %v", tt.engine, tt.version, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDBEngineOnly(t *testing.T) {
	tests := []struct {
		engine  string
		wantErr bool
	}{
		{"postgresql", false},
		{"mysql", false},
		{"mariadb", false},
		{"redis", false},
		{"mongodb", false},
		{"postgres", false},
		{"POSTGRESQL", false},
		{"", true},
		{"unknown", true},
		{"sqlite", true},
	}

	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			err := ValidateDBEngineOnly(tt.engine)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDBEngineOnly(%q) error = %v, wantErr %v", tt.engine, err, tt.wantErr)
			}
		})
	}
}

func TestSupportedDBEngines(t *testing.T) {
	engines := SupportedDBEngines
	if len(engines) != 5 {
		t.Errorf("expected 5 supported engines, got %d", len(engines))
	}

	for _, engine := range []string{"postgresql", "mysql", "mariadb", "redis", "mongodb"} {
		versions, ok := engines[engine]
		if !ok {
			t.Errorf("expected engine %q to be supported", engine)
		}
		if len(versions) == 0 {
			t.Errorf("engine %q has no versions", engine)
		}
	}

	pgVersions := engines["postgresql"]
	found := false
	for _, v := range pgVersions {
		if v == "16" {
			found = true
			break
		}
	}
	if !found {
		t.Error("postgresql 16 not found in supported versions")
	}
}

func TestDBEngineDefaultPorts(t *testing.T) {
	tests := []struct {
		engine string
		port   int
	}{
		{"postgresql", 5432},
		{"mysql", 3306},
		{"mariadb", 3306},
		{"redis", 6379},
		{"mongodb", 27017},
	}
	for _, tt := range tests {
		if port := DBEngineDefaultPorts[tt.engine]; port != tt.port {
			t.Errorf("default port for %s: got %d, want %d", tt.engine, port, tt.port)
		}
	}
}

func TestDBEngineImages(t *testing.T) {
	tests := []struct {
		engine string
		image  string
	}{
		{"postgresql", "postgres"},
		{"mysql", "mysql"},
		{"mariadb", "mariadb"},
		{"redis", "redis"},
		{"mongodb", "mongo"},
	}
	for _, tt := range tests {
		if image := DBEngineImages[tt.engine]; image != tt.image {
			t.Errorf("image for %s: got %s, want %s", tt.engine, image, tt.image)
		}
	}
}
