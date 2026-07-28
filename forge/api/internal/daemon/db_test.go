package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDatabaseBackupRestoreAndStatusContracts(t *testing.T) {
	const backupID = "123e4567-e89b-12d3-a456-426614174000"
	var sawBackup, sawRestore, sawStatus bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/database/backup":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["containerId"] != "container-1" || body["engine"] != "postgresql" || body["backupId"] != backupID {
				t.Errorf("unexpected backup request: %#v", body)
			}
			sawBackup = true
			_ = json.NewEncoder(w).Encode(DatabaseBackupResponse{
				OK: true, BackupID: backupID, File: "/private/backup", Name: backupID + ".backup.gz", Engine: "postgresql", Size: 42, Checksum: "abc",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/database/restore":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["containerId"] != "container-1" || body["engine"] != "postgresql" || body["backupId"] != backupID {
				t.Errorf("unexpected restore request: %#v", body)
			}
			sawRestore = true
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.Method == http.MethodGet && r.URL.Path == "/database/status/container-1":
			sawStatus = true
			_ = json.NewEncoder(w).Encode(DatabaseStatusResponse{Status: "running", Running: true, ContainerID: "container-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "node.secret")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := client.BackupDatabase(context.Background(), server.URL, "node.secret", "container-1", "postgresql", backupID)
	if err != nil || entry.BackupID != backupID || entry.Size != 42 {
		t.Fatalf("backup response=%#v err=%v", entry, err)
	}
	if err := client.RestoreDatabase(context.Background(), server.URL, "node.secret", "container-1", "postgresql", backupID); err != nil {
		t.Fatal(err)
	}
	status, err := client.DatabaseStatus(context.Background(), server.URL, "node.secret", "container-1")
	if err != nil || !status.Running || status.Status != "running" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	if !sawBackup || !sawRestore || !sawStatus {
		t.Fatalf("missing calls: backup=%t restore=%t status=%t", sawBackup, sawRestore, sawStatus)
	}
}
