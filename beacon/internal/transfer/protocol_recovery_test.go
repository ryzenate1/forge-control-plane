package transfer

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"
)

func TestProtocolRecoveryAfterCrash(t *testing.T) {
	dataDir := t.TempDir()
	// First engine simulates pre-crash instance
	engine1, err := NewProtocolEngine(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	registerProtocolCredential(t, engine1, DirectionDestinationUpload, "crash-token")
	payload := makeTarGz(t, map[string]string{"hello.txt": "hello world"})
	checksum := shaHex(payload)
	// Partial upload before crash
	partial, err := engine1.AppendDestination(context.Background(), "migration-1", "crash-token", 0, int64(len(payload)), checksum, bytes.NewReader(payload[:8]))
	if err != nil {
		t.Fatal(err)
	}
	if partial.Offset != 8 {
		t.Fatalf("partial offset=%d want 8", partial.Offset)
	}
	// Simulate crash: discard engine1 without cleanup, create new engine from same dataDir
	engine2, err := NewProtocolEngine(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	// New engine should recover offset from journal
	recovered, err := engine2.DestinationOffset("migration-1", "crash-token")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Offset != 8 {
		t.Fatalf("recovered offset=%d want 8", recovered.Offset)
	}
	if recovered.Phase != "uploading" {
		t.Fatalf("recovered phase=%q want uploading", recovered.Phase)
	}
	// Continue upload after recovery
	complete, err := engine2.AppendDestination(context.Background(), "migration-1", "crash-token", recovered.Offset, int64(len(payload)), checksum, bytes.NewReader(payload[8:]))
	if err != nil {
		t.Fatal(err)
	}
	if complete.Phase != "verified" || complete.Offset != int64(len(payload)) {
		t.Fatalf("complete after recovery: %+v", complete)
	}
	// Restore should still work after recovery
	if _, err := engine2.RestoreDestination(context.Background(), "migration-1", "crash-token"); err != nil {
		t.Fatal(err)
	}
	if err := engine2.FinalizeDestination("migration-1", "crash-token"); err != nil {
		t.Fatal(err)
	}
}

func TestProtocolJournalSurvivesCrashMidPrepare(t *testing.T) {
	dataDir := t.TempDir()
	root := dataDir + "/server-1"
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/keep.txt", []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	engine1, err := NewProtocolEngine(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	claims := CredentialClaims{
		Version: ProtocolVersion, MigrationID: "migration-2", ServerID: "server-1",
		SourceNodeID: "source-1", TargetNodeID: "target-1", Direction: DirectionSourceControl,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	if err := engine1.Register(CredentialRegistration{Claims: claims, CredentialHash: HashCredential("source-token")}); err != nil {
		t.Fatal(err)
	}
	// Prepare source archive before crash
	meta1, err := engine1.PrepareSource(context.Background(), "migration-2", "source-token")
	if err != nil {
		t.Fatal(err)
	}
	if meta1.Phase != "archived" {
		t.Fatalf("prepare phase=%q want archived", meta1.Phase)
	}
	// Simulate crash and recovery
	engine2, err := NewProtocolEngine(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	meta2, err := engine2.Authorize("migration-2", DirectionSourceControl, "source-token")
	if err != nil {
		t.Fatal(err)
	}
	if meta2.Phase != "archived" || meta2.Checksum == "" || meta2.ArchiveSize == 0 {
		t.Fatalf("recovered source meta not persisted: %+v", meta2)
	}
	// Source archive should still be readable after crash
	if _, _, err := engine2.SourceArchive("migration-2", "source-token", 0); err != nil {
		t.Fatal(err)
	}
}
