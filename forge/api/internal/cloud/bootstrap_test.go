package cloud

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestBeaconCloudInitContainsHardenedRuntimeBootstrap(t *testing.T) {
	data, err := BeaconCloudInit("node-1", "id.secret", "https://panel.example.com/api/v1", "ghcr.io/acme/beacon:v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"download.docker.com/linux/ubuntu", "--restart unless-stopped", "DAEMON_ALLOW_INSECURE_NO_AUTH=false", "/var/run/docker.sock", "ghcr.io/acme/beacon:v1"} {
		if !strings.Contains(data, expected) {
			t.Fatalf("cloud-init missing %q", expected)
		}
	}
	if strings.Contains(data, "id.secret") {
		t.Fatal("credential must not appear in plaintext cloud-init script")
	}
}

func TestBeaconCloudInitIncludesSharedS3BackupConfiguration(t *testing.T) {
	data, err := BeaconCloudInit("node-1", "id.secret", "https://panel.example.com/api/v1", "ghcr.io/acme/beacon:v1", BeaconBackupConfig{
		Adapter: "s3", Bucket: "gamepanel-backups", Region: "ap-south-1", Prefix: "production", UsePathStyle: "false",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"BACKUP_ADAPTER", "S3_BUCKET", base64.StdEncoding.EncodeToString([]byte("gamepanel-backups"))} {
		if !strings.Contains(data, expected) {
			t.Fatalf("cloud-init missing %q", expected)
		}
	}
	if _, err := BeaconCloudInit("node-1", "id.secret", "https://panel.example.com/api/v1", "ghcr.io/acme/beacon:v1", BeaconBackupConfig{Adapter: "s3"}); err == nil {
		t.Fatal("expected missing S3 bucket error")
	}
}

func TestBeaconCloudInitRejectsInvalidInputs(t *testing.T) {
	if _, err := BeaconCloudInit("node", "token", "file:///tmp/panel", "image"); err == nil {
		t.Fatal("expected invalid URL error")
	}
	if _, err := BeaconCloudInit("node", "token", "https://panel.example", "bad image"); err == nil {
		t.Fatal("expected invalid image error")
	}
}
