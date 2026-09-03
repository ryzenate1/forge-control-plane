package backup_test

import (
	"context"
	"os"
	"testing"
	"time"

	"gamepanel/beacon/internal/backup"
	"github.com/go-co-op/gocron"
	"github.com/stretchr/testify/assert"
)

func TestScheduler_ScheduleAndCancel(t *testing.T) {
	cron := gocron.NewScheduler(time.UTC)
	defer cron.Stop()

	adapter := backup.NewMockBackup()
	scheduler := backup.NewScheduler(nil, cron)
	scheduler.RegisterAdapter("mock", adapter)

	serverID := "test-server"
	err := scheduler.Schedule(serverID, t.TempDir(), "0 0 * * *", "mock")
	assert.NoError(t, err)

	err = scheduler.Cancel(serverID)
	assert.NoError(t, err)
}

func TestScheduler_RunBackup(t *testing.T) {
	cron := gocron.NewScheduler(time.UTC)
	defer cron.Stop()

	adapter := backup.NewMockBackup()
	scheduler := backup.NewScheduler(nil, cron)
	scheduler.RegisterAdapter("mock", adapter)

	ctx := context.Background()
	err := scheduler.RunBackup(ctx, "test-server", t.TempDir(), "mock")
	assert.NoError(t, err)
}

func TestScheduler_UnknownAdapter(t *testing.T) {
	cron := gocron.NewScheduler(time.UTC)
	defer cron.Stop()

	scheduler := backup.NewScheduler(nil, cron)

	err := scheduler.Schedule("s1", t.TempDir(), "0 0 * * *", "nonexistent")
	assert.Error(t, err)

	err = scheduler.RunBackup(context.Background(), "s1", t.TempDir(), "nonexistent")
	assert.Error(t, err)

	err = scheduler.Cancel("s1")
	assert.Error(t, err)
}

func TestScheduler_DuplicateSchedule(t *testing.T) {
	cron := gocron.NewScheduler(time.UTC)
	defer cron.Stop()

	adapter := backup.NewMockBackup()
	scheduler := backup.NewScheduler(nil, cron)
	scheduler.RegisterAdapter("mock", adapter)

	root := t.TempDir()
	err := scheduler.Schedule("s1", root, "0 0 * * *", "mock")
	assert.NoError(t, err)

	err = scheduler.Schedule("s1", root, "0 30 * * *", "mock")
	assert.Error(t, err)
}

func TestSchedulerRejectsMissingServerRoot(t *testing.T) {
	cron := gocron.NewScheduler(time.UTC)
	defer cron.Stop()
	scheduler := backup.NewScheduler(nil, cron)
	scheduler.RegisterAdapter("mock", backup.NewMockBackup())

	err := scheduler.Schedule("s1", t.TempDir()+string(os.PathSeparator)+"missing", "0 0 * * *", "mock")
	assert.Error(t, err)
}
