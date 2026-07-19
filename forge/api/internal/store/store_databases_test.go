package store

import (
	"strings"
	"testing"
)

func TestNormalizeDatabaseEngineStrict(t *testing.T) {
	tests := map[string]string{
		"postgres":   "postgresql",
		"PostgreSQL": "postgresql",
		"mysql":      "mysql",
		"MariaDB":    "mysql",
	}
	for input, want := range tests {
		got, err := normalizeDatabaseEngine(input)
		if err != nil || got != want {
			t.Fatalf("normalizeDatabaseEngine(%q) = %q, %v", input, got, err)
		}
	}
	for _, input := range []string{"", "sqlite", "postgresql;DROP TABLE x"} {
		if _, err := normalizeDatabaseEngine(input); err == nil {
			t.Fatalf("normalizeDatabaseEngine(%q) accepted unsupported engine", input)
		}
	}
}

func TestDatabaseHostValidationDefaultsToVerifiedTLS(t *testing.T) {
	engine, name, host, username := "mysql", "primary", "db.example.com", "root-admin"
	port := 0
	tlsMode, tlsCA, serverName := "", "", ""
	if err := validateDatabaseHostRequest(&engine, &name, &host, &port, &username, &tlsMode, &tlsCA, &serverName, nil); err != nil {
		t.Fatal(err)
	}
	if port != 3306 || tlsMode != "verify-full" {
		t.Fatalf("port=%d tlsMode=%q", port, tlsMode)
	}
}

func TestDatabaseHostValidationRejectsUnsafeValues(t *testing.T) {
	tests := []struct {
		name, engine, host, username, tlsMode string
		port                                  int
	}{
		{name: "engine", engine: "sqlite", host: "db.example.com", username: "admin", tlsMode: "required", port: 5432},
		{name: "host injection", engine: "mysql", host: "db.example.com:3306)/evil", username: "admin", tlsMode: "required", port: 3306},
		{name: "username injection", engine: "postgres", host: "127.0.0.1", username: `admin";DROP`, tlsMode: "required", port: 5432},
		{name: "port", engine: "mysql", host: "::1", username: "admin", tlsMode: "required", port: 65536},
		{name: "tls", engine: "mysql", host: "db.example.com", username: "admin", tlsMode: "prefer", port: 3306},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, ca, serverName := "host", "", ""
			if err := validateDatabaseHostRequest(&tt.engine, &name, &tt.host, &tt.port, &tt.username, &tt.tlsMode, &ca, &serverName, nil); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDatabaseHostForConnectionTestNormalizesWithoutPersistence(t *testing.T) {
	host, password, err := DatabaseHostForConnectionTest(CreateDatabaseHostRequest{
		Engine:   "MariaDB",
		Name:     " primary ",
		Host:     " db.example.com ",
		Username: " admin ",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if password != "secret" || host.Engine != "mysql" || host.Name != "primary" || host.Host != "db.example.com" || host.Username != "admin" || host.Port != 3306 || host.TLSMode != "verify-full" {
		t.Fatalf("unexpected normalized test host: %#v, password=%q", host, password)
	}
}

func TestDatabaseHostValidationRejectsInvalidMaxDatabases(t *testing.T) {
	for _, limit := range []int{-1, 0} {
		engine, name, host, username := "postgresql", "primary", "db.example.com", "admin"
		port := 5432
		tlsMode, tlsCA, serverName := "verify-full", "", ""
		if err := validateDatabaseHostRequest(&engine, &name, &host, &port, &username, &tlsMode, &tlsCA, &serverName, &limit); err == nil {
			t.Fatalf("maxDatabases=%d was accepted", limit)
		}
	}
}

func TestDatabaseIdentifierAndRemoteValidation(t *testing.T) {
	for _, valid := range []string{"Game", "game_2", "A123"} {
		if _, err := validateRequestedDatabaseIdentifier(valid); err != nil {
			t.Fatalf("valid identifier %q rejected: %v", valid, err)
		}
	}
	for _, invalid := range []string{"2game", "game-name", `game"name`, "game name", strings.Repeat("a", 25)} {
		if _, err := validateRequestedDatabaseIdentifier(invalid); err == nil {
			t.Fatalf("unsafe identifier %q accepted", invalid)
		}
	}
	for _, valid := range []string{"%", "10.0.0.8", "2001:db8::1", "client.example.com"} {
		if _, err := ValidateDatabaseRemote(valid); err != nil {
			t.Fatalf("valid remote %q rejected: %v", valid, err)
		}
	}
	for _, invalid := range []string{"10.%", "*.example.com", `host'@'%'`, "host/name"} {
		if _, err := ValidateDatabaseRemote(invalid); err == nil {
			t.Fatalf("unsafe remote %q accepted", invalid)
		}
	}
}

func TestGlobalDatabaseHostsAreSelectableAndScannable(t *testing.T) {
	if !strings.Contains(databaseHostSelectionSQL, "NOT EXISTS (SELECT 1 FROM database_host_node dhn WHERE dhn.database_host_id = h.id)") {
		t.Fatal("global database hosts are missing from server host selection")
	}
	if !strings.Contains(databaseHostSelectionSQL, "ORDER BY (NOT EXISTS (SELECT 1 FROM database_host_node dhn WHERE dhn.database_host_id = h.id))") {
		t.Fatal("node-local hosts should be preferred before global hosts")
	}
	if !strings.Contains(databaseHostSelect, "COALESCE(array_remove(array_agg(DISTINCT dhn.node_id::text), NULL), '{}')") || !strings.Contains(databaseHostSelect, "LEFT JOIN nodes") {
		t.Fatal("global database host list/get query cannot safely scan a NULL node")
	}
}
