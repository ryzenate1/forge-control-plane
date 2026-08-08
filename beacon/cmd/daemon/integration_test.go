//go:build integration

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gamepanel/beacon/config"
	"gamepanel/beacon/internal/progress"
	"gamepanel/beacon/internal/quota"
	"gamepanel/beacon/internal/server"
	"gamepanel/beacon/internal/sftpserver"
)

func TestProgressTracking(t *testing.T) {
	reports := make([]progress.Report, 0)
	p := progress.New(func(r progress.Report) {
		reports = append(reports, r)
	})

	p.SetTotal(1000)
	p.SetPhase(progress.PhaseArchiving)
	p.Add(500)

	r := p.Report()
	assert.Equal(t, int64(500), r.BytesProcessed)
	assert.Equal(t, int64(1000), r.TotalBytes)
	assert.Equal(t, progress.PhaseArchiving, r.Phase)

	p.Add(500)
	p.SetPhase(progress.PhaseCompleted)

	r = p.Report()
	assert.Equal(t, int64(1000), r.BytesProcessed)
	assert.Equal(t, progress.PhaseCompleted, r.Phase)
	assert.GreaterOrEqual(t, len(reports), 1)
}

func TestQuotaNoopTracker(t *testing.T) {
	tracker := quota.NoopTracker{}

	_, err := tracker.GetUsage(context.Background(), t.TempDir())
	assert.ErrorIs(t, err, quota.ErrNoQuota)

	err = tracker.Enforce(context.Background(), "/tmp", 100, 50, 200)
	assert.NoError(t, err)

	err = tracker.Enforce(context.Background(), "/tmp", 100, 150, 200)
	assert.NoError(t, err, "NoopTracker should not enforce quotas")

	err = tracker.Enforce(context.Background(), "/tmp", 1000, 5000, 0)
	assert.NoError(t, err)

	err = tracker.Enforce(context.Background(), "/tmp", 1000, 5000, -1)
	assert.NoError(t, err)

	err = tracker.SetQuota("/tmp", 1024)
	assert.NoError(t, err)
}

func TestSFTPEvents(t *testing.T) {
	event := sftpserver.Event{
		Action:    sftpserver.ActionFileRead,
		Path:      "/test/file.txt",
		UserID:    "user1",
		ServerID:  "srv1",
		IP:        "127.0.0.1",
		SessionID: "sess1",
	}
	assert.Equal(t, sftpserver.ActionFileRead, event.Action)
	assert.Contains(t, event.String(), "sftp.file.read")
	assert.Contains(t, event.String(), "/test/file.txt")

	received := make([]sftpserver.Event, 0)
	publisher := sftpserver.NewMultiPublisher(
		sftpserver.PublisherFunc(func(e sftpserver.Event) {
			received = append(received, e)
		}),
	)
	publisher.PublishEvent(event)
	assert.Len(t, received, 1)
	assert.Equal(t, event.Path, received[0].Path)
	assert.Equal(t, event.UserID, received[0].UserID)

	publisher.PublishEvent(event)
	assert.Len(t, received, 2)
}

func TestConfigExpand(t *testing.T) {
	t.Setenv("TEST_BEACON_VAL", "hello")
	result, err := config.Expand("prefix-$TEST_BEACON_VAL-suffix")
	assert.NoError(t, err)
	assert.Equal(t, "prefix-hello-suffix", result)

	dir := t.TempDir()
	filePath := filepath.Join(dir, "secret.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("my-secret-value\n"), 0o600))
	result, err = config.Expand("file://" + filePath)
	assert.NoError(t, err)
	assert.Equal(t, "my-secret-value", result)

	_, err = config.Expand("file:///nonexistent/file.txt")
	assert.Error(t, err)
}

func TestThrottleConfig(t *testing.T) {
	cfg := config.ConsoleThrottles{
		Enabled: true,
		Lines:   1000,
		Period:  200,
	}
	assert.True(t, cfg.Enabled)
	assert.Equal(t, uint64(1000), cfg.Lines)
	assert.Equal(t, uint64(200), cfg.Period)

	cfg.Enabled = false
	assert.False(t, cfg.Enabled)
}

func TestCapabilities(t *testing.T) {
	entry := server.CapabilityEntry{
		Type:    server.CapabilityRuntime,
		Version: "1.0.0",
		Status:  "ok",
	}
	assert.Equal(t, server.CapabilityRuntime, entry.Type)
	assert.Equal(t, "ok", entry.Status)

	compat := server.CheckVersionCompatibility("1.0.0", "v1")
	assert.True(t, compat.Compatible)
	assert.Equal(t, "1.0.0", compat.BeaconVersion)

	compat = server.CheckVersionCompatibility("", "v1")
	assert.False(t, compat.Compatible)

	caps := []server.CapabilityType{
		server.CapabilityRuntime,
		server.CapabilityBuild,
		server.CapabilityCompose,
		server.CapabilityStorage,
		server.CapabilityGateway,
		server.CapabilityDatabase,
	}
	for _, c := range caps {
		assert.NotEmpty(t, string(c), "capability type should be non-empty")
	}
}
