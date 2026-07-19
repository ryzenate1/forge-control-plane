package backup_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gamepanel/beacon/internal/backup"
)

func TestVerifyBackup(t *testing.T) {
	adapter := backup.NewMockBackup()

	ctx := context.Background()
	info, err := adapter.Create(ctx, "/test", "server-1", "test-backup.zip", nil)
	require.NoError(t, err)

	result, err := backup.VerifyBackup(ctx, adapter, "server-1", info.Name)
	assert.NoError(t, err)
	assert.Equal(t, backup.BackupStatusCompleted, result.Status)
	assert.Equal(t, info.Name, result.BackupID)
	assert.NotEmpty(t, result.Checksum)
}
