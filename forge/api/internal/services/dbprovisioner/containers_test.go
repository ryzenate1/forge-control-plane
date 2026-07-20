package dbprovisioner

import (
	"encoding/json"
	"testing"
)

func TestGeneratePassword(t *testing.T) {
	pw1 := generatePassword(32)
	pw2 := generatePassword(32)
	if pw1 == pw2 {
		t.Error("generated passwords should be different")
	}
	if len(pw1) != 32 {
		t.Errorf("generated password length = %d, want 32", len(pw1))
	}
}

func TestGenerateDBName(t *testing.T) {
	name1 := generateDBName()
	name2 := generateDBName()
	if name1 == name2 {
		t.Error("generated db names should be different")
	}
	if len(name1) < 3 {
		t.Error("generated db name too short")
	}
}

func TestGenerateUsername(t *testing.T) {
	u1 := generateUsername()
	u2 := generateUsername()
	if u1 == u2 {
		t.Error("generated usernames should be different")
	}
	if len(u1) < 3 {
		t.Error("generated username too short")
	}
}

func TestImageForDB(t *testing.T) {
	tests := []struct {
		engine  string
		version string
		want    string
	}{
		{"postgresql", "16", "postgres:16"},
		{"mysql", "8.0", "mysql:8.0"},
		{"mariadb", "11", "mariadb:11"},
		{"redis", "7", "redis:7"},
		{"mongodb", "7", "mongo:7"},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			got := imageForDB(tt.engine, tt.version)
			if got != tt.want {
				t.Errorf("imageForDB(%q, %q) = %q, want %q", tt.engine, tt.version, got, tt.want)
			}
		})
	}
}

func TestEnvVarsForDB(t *testing.T) {
	tests := []struct {
		engine   string
		dbName   string
		username string
		password string
		contains []string
	}{
		{
			engine:   "postgresql",
			dbName:   "testdb",
			username: "testuser",
			password: "secret",
			contains: []string{"POSTGRES_DB=testdb", "POSTGRES_USER=testuser", "POSTGRES_PASSWORD=secret"},
		},
		{
			engine:   "mysql",
			dbName:   "testdb",
			username: "testuser",
			password: "secret",
			contains: []string{"MYSQL_DATABASE=testdb", "MYSQL_USER=testuser", "MYSQL_PASSWORD=secret", "MYSQL_ROOT_PASSWORD=secret"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			env := envVarsForDB(tt.engine, tt.dbName, tt.username, tt.password)
			for _, want := range tt.contains {
				found := false
				for _, e := range env {
					if e == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("envVarsForDB(%q) missing %q in %v", tt.engine, want, env)
				}
			}
		})
	}
}

func TestConnectionStringForDB(t *testing.T) {
	t.Run("postgresql", func(t *testing.T) {
		cs := connectionStringForDB("postgresql", "testdb", "user", "pass", "localhost", 5432, "16")
		if cs != "postgresql://user:pass@localhost:5432/testdb?sslmode=disable" {
			t.Errorf("unexpected connection string: %s", cs)
		}
	})
	t.Run("mysql", func(t *testing.T) {
		cs := connectionStringForDB("mysql", "testdb", "user", "pass", "localhost", 3306, "8.0")
		if cs != "mysql://user:pass@localhost:3306/testdb" {
			t.Errorf("unexpected connection string: %s", cs)
		}
	})
	t.Run("redis", func(t *testing.T) {
		cs := connectionStringForDB("redis", "", "", "pass", "localhost", 6379, "7")
		if cs != "redis://:pass@localhost:6379" {
			t.Errorf("unexpected connection string: %s", cs)
		}
	})
}

func TestCredentialsJSON(t *testing.T) {
	raw, err := credentialsJSON("postgresql", "testdb", "user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	var creds map[string]string
	if err := json.Unmarshal(raw, &creds); err != nil {
		t.Fatal(err)
	}
	if creds["username"] != "user" || creds["password"] != "pass" || creds["database"] != "testdb" {
		t.Errorf("unexpected credentials: %v", creds)
	}
}

func TestDefaultPortForEngine(t *testing.T) {
	tests := []struct {
		engine string
		port   int
	}{
		{"postgresql", 5432},
		{"mysql", 3306},
		{"mariadb", 3306},
		{"redis", 6379},
		{"mongodb", 27017},
		{"unknown", 0},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			if got := defaultPortForEngine(tt.engine); got != tt.port {
				t.Errorf("defaultPortForEngine(%q) = %d, want %d", tt.engine, got, tt.port)
			}
		})
	}
}

func TestSupportedEngines(t *testing.T) {
	svc := &DBContainerService{}
	engines := svc.SupportedEngines()
	if len(engines) != 5 {
		t.Errorf("expected 5 engines, got %d", len(engines))
	}
}
