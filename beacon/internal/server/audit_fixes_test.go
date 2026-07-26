package server

import (
	"archive/tar"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirewallStatePersistsAndReconciles(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "firewall.json")
	t.Setenv("BEACON_FIREWALL_STATE_PATH", statePath)
	previousExec := firewallExec
	t.Cleanup(func() { firewallExec = previousExec })
	var commands [][]string
	firewallExec = func(_ context.Context, name string, args ...string) error {
		commands = append(commands, append([]string{name}, args...))
		return nil
	}
	data := &firewallData{state: firewallState{
		Enabled: true,
		Rules: map[string]FirewallRule{
			"rule-1": {ID: "rule-1", Port: 443, Protocol: "tcp", Action: "allow"},
		},
		Forwards: map[string]PortForward{
			"fwd-1": {ID: "fwd-1", FromPort: 8443, ToPort: 443, ToIP: "127.0.0.1", Protocol: "tcp"},
		},
	}}
	data.mu.Lock()
	if err := data.persistLocked(); err != nil {
		data.mu.Unlock()
		t.Fatal(err)
	}
	data.mu.Unlock()

	loaded := &firewallData{state: firewallState{}}
	if err := loaded.load(); err != nil {
		t.Fatal(err)
	}
	if len(loaded.state.Rules) != 1 || len(loaded.state.Forwards) != 1 {
		t.Fatalf("firewall state did not survive restart: %+v", loaded.state)
	}
	if err := loaded.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(commands) < 4 {
		t.Fatalf("expected chain and rule reconciliation commands, got %v", commands)
	}
}

func TestFirewallRuleArgsContainNoShell(t *testing.T) {
	rule := FirewallRule{ID: "rule-1", Port: 25565, Protocol: "tcp", SourceIP: "192.0.2.0/24"}
	args := ruleArgs("-A", rule)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--comment forge:rule-1") || strings.ContainsAny(joined, ";`$") {
		t.Fatalf("unsafe or incomplete firewall args: %v", args)
	}
}

func TestComposeIdentifiersAndEnvironmentEncoding(t *testing.T) {
	for _, invalid := range []string{"", "../escape", "a/b", ".", "space name", "-leading", "Uppercase", strings.Repeat("a", 129)} {
		if validStackID(invalid) {
			t.Fatalf("expected stack id %q to be rejected", invalid)
		}
	}
	content, err := encodeComposeEnv(map[string]string{
		"SAFE_KEY": "line1\nINJECTED=value",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "\nINJECTED=") {
		t.Fatalf("environment value injected a second assignment: %q", content)
	}
	if _, err := encodeComposeEnv(map[string]string{"BAD-KEY": "value"}); err == nil {
		t.Fatal("expected invalid environment key to be rejected")
	}
	if err := validateComposePolicy("services:\n  app:\n    image: alpine\n    privileged: true\n"); err == nil {
		t.Fatal("expected privileged compose service to be rejected")
	}
	if err := validateComposePolicy("services:\n  app:\n    image: alpine\n    volumes:\n      - /etc:/host\n"); err == nil {
		t.Fatal("expected host bind mount to be rejected")
	}
}

func TestFirewallValidation(t *testing.T) {
	if _, err := validateFirewallProtocol("tcp/udp"); err == nil {
		t.Fatal("expected combined protocol to be rejected")
	}
	if _, err := validateFirewallAction("REJECT"); err == nil {
		t.Fatal("expected unsupported action to be rejected")
	}
	if err := validateFirewallSource("127.0.0.1;--jump ACCEPT"); err == nil {
		t.Fatal("expected argument injection source to be rejected")
	}
	if err := validateForwardIP("10.0.0.5 --to-ports 22"); err == nil {
		t.Fatal("expected argument injection destination to be rejected")
	}
}

func TestDatabaseEngineCommands(t *testing.T) {
	if validDatabaseEngine("unknown") {
		t.Fatal("unknown database engine should be rejected")
	}
	redis := databaseCommand("redis", "secret")
	if len(redis) == 0 || !strings.Contains(strings.Join(redis, " "), "--requirepass") {
		t.Fatalf("redis command does not enforce authentication: %v", redis)
	}
	if !strings.Contains(strings.Join(backupCommandForEngine("mongodb"), " "), "--authenticationDatabase admin") {
		t.Fatal("mongodb backup does not authenticate")
	}
	for _, engine := range []string{"postgresql", "mysql", "mariadb", "mongodb", "redis"} {
		command := restoreCommandForEngine(engine, "/tmp/backup.backup.gz")
		if command == "" || command == "false" || !strings.Contains(command, "gzip -dc") {
			t.Fatalf("%s restore command is not implemented: %q", engine, command)
		}
	}
}

func TestDatabaseBackupHelpers(t *testing.T) {
	for _, valid := range []string{"backup-123", "ABC_def", "123e4567-e89b-12d3-a456-426614174000"} {
		if !validBackupID(valid) {
			t.Fatalf("expected backup id %q to be valid", valid)
		}
	}
	for _, invalid := range []string{"", "../escape", "a/b", "space name", strings.Repeat("a", 129)} {
		if validBackupID(invalid) {
			t.Fatalf("expected backup id %q to be rejected", invalid)
		}
	}

	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	payload := []byte("database backup")
	if err := tw.WriteHeader(&tar.Header{Name: "tmp/mgp-backup.sql.gz", Mode: 0o600, Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var extracted bytes.Buffer
	size, err := copyFileFromTar(&extracted, bytes.NewReader(archive.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(payload)) || !bytes.Equal(extracted.Bytes(), payload) {
		t.Fatalf("unexpected extraction size=%d payload=%q", size, extracted.Bytes())
	}
}

func TestDatabaseBackupDownloadAndDelete(t *testing.T) {
	t.Setenv("DAEMON_DATA_DIR", t.TempDir())
	dir, err := databaseBackupDir()
	if err != nil {
		t.Fatal(err)
	}
	const backupID = "db-backup-123"
	payload := []byte("persistent database backup")
	path := filepath.Join(dir, backupID+".backup.gz")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	server := &Server{}
	downloadRequest := httptest.NewRequest(http.MethodGet, "/database/backups/"+backupID, nil)
	downloadRequest.SetPathValue("backupId", backupID)
	downloadResponse := httptest.NewRecorder()
	server.handleDatabaseBackupDownload(downloadResponse, downloadRequest)
	if downloadResponse.Code != http.StatusOK || !bytes.Equal(downloadResponse.Body.Bytes(), payload) {
		t.Fatalf("download status=%d payload=%q", downloadResponse.Code, downloadResponse.Body.Bytes())
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/database/backups/"+backupID, nil)
	deleteRequest.SetPathValue("backupId", backupID)
	deleteResponse := httptest.NewRecorder()
	server.handleDatabaseBackupDelete(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("backup file still exists after delete: %v", err)
	}

	missingResponse := httptest.NewRecorder()
	server.handleDatabaseBackupDownload(missingResponse, downloadRequest)
	if missingResponse.Code != http.StatusNotFound {
		t.Fatalf("missing download status=%d", missingResponse.Code)
	}
}
