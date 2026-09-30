package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyChecksumManifestSignature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifest := filepath.Join(dir, "checksums.txt")
	signature := filepath.Join(dir, "checksums.txt.sig")
	payload := []byte(strings.Repeat("a", 64) + "  beacon_linux_amd64\n")
	if err := os.WriteFile(manifest, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(signature, []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := UpdatePublicKey
	UpdatePublicKey = base64.StdEncoding.EncodeToString(publicKey)
	t.Cleanup(func() { UpdatePublicKey = previous })
	if err := verifyChecksumManifestSignature(manifest, signature); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, append(payload, 'x'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksumManifestSignature(manifest, signature); err == nil {
		t.Fatal("tampered manifest was accepted")
	}
}

func TestCompareVersionsUsesSemanticOrdering(t *testing.T) {
	tests := []struct {
		left, right string
		want        int
	}{
		{"v1.10.0", "v1.9.9", 1},
		{"v2.0.0-rc.1", "v2.0.0", -1},
		{"v2.0.0-rc.10", "v2.0.0-rc.2", 1},
		{"1.0.0+build.1", "v1.0.0+build.2", 0},
	}
	for _, test := range tests {
		got, err := compareVersions(test.left, test.right)
		if err != nil {
			t.Fatalf("compare %q and %q: %v", test.left, test.right, err)
		}
		if got != test.want {
			t.Fatalf("compare %q and %q = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestExpectedChecksumRequiresExactFilenameAndSHA256(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "checksums.txt")
	expected := strings.Repeat("a", 64)
	if err := os.WriteFile(manifest, []byte(expected+"  other-beacon_linux_amd64\n"+expected+"  beacon_linux_amd64\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := expectedChecksum(manifest, "beacon_linux_amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got != expected {
		t.Fatalf("checksum = %q, want %q", got, expected)
	}
}

func TestInstallUpdateAtomicallyRetainsRollbackCopy(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "beacon")
	download := filepath.Join(dir, "download")
	if err := os.WriteFile(current, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("new-binary")
	if err := os.WriteFile(download, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	if err := installUpdateAtomically(download, current, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(current)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("installed binary = %q", got)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".beacon.rollback-*"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("rollback files = %v, err = %v", matches, err)
	}
	rollback, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(rollback) != "old-binary" {
		t.Fatalf("rollback binary = %q", rollback)
	}
}
