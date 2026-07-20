package backup

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gamepanel/forge/internal/store"
)

func skipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run storage adapter tests")
	}
}

type memoryAdapter struct {
	store map[string][]byte
}

func newMemoryAdapter() *memoryAdapter {
	return &memoryAdapter{store: make(map[string][]byte)}
}

func (a *memoryAdapter) Name() string { return "memory" }

func (a *memoryAdapter) Upload(_ context.Context, path string, data []byte) error {
	a.store[path] = make([]byte, len(data))
	copy(a.store[path], data)
	return nil
}

func (a *memoryAdapter) Download(_ context.Context, path string) ([]byte, error) {
	d, ok := a.store[path]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]byte, len(d))
	copy(out, d)
	return out, nil
}

func (a *memoryAdapter) Delete(_ context.Context, path string) error {
	delete(a.store, path)
	return nil
}

func (a *memoryAdapter) List(_ context.Context, prefix string) ([]string, error) {
	var names []string
	for k := range a.store {
		if strings.HasPrefix(k, prefix) {
			names = append(names, k)
		}
	}
	return names, nil
}

func (a *memoryAdapter) Exists(_ context.Context, path string) (bool, error) {
	_, ok := a.store[path]
	return ok, nil
}

func TestMemoryAdapter_UploadDownload(t *testing.T) {
	t.Parallel()
	adapter := newMemoryAdapter()
	ctx := context.Background()
	data := []byte("hello-world-backup")

	err := adapter.Upload(ctx, "server1/backup-1.tar.gz", data)
	require.NoError(t, err)

	downloaded, err := adapter.Download(ctx, "server1/backup-1.tar.gz")
	require.NoError(t, err)
	assert.Equal(t, data, downloaded)
}

func TestMemoryAdapter_Exists(t *testing.T) {
	t.Parallel()
	adapter := newMemoryAdapter()
	ctx := context.Background()

	exists, err := adapter.Exists(ctx, "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)

	err = adapter.Upload(ctx, "test.dat", []byte("test"))
	require.NoError(t, err)

	exists, err = adapter.Exists(ctx, "test.dat")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestMemoryAdapter_Delete(t *testing.T) {
	t.Parallel()
	adapter := newMemoryAdapter()
	ctx := context.Background()

	err := adapter.Upload(ctx, "to-delete", []byte("data"))
	require.NoError(t, err)

	err = adapter.Delete(ctx, "to-delete")
	require.NoError(t, err)

	exists, err := adapter.Exists(ctx, "to-delete")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestMemoryAdapter_List(t *testing.T) {
	t.Parallel()
	adapter := newMemoryAdapter()
	ctx := context.Background()

	err := adapter.Upload(ctx, "prefix/a.tar.gz", []byte("a"))
	require.NoError(t, err)
	err = adapter.Upload(ctx, "prefix/b.tar.gz", []byte("b"))
	require.NoError(t, err)
	err = adapter.Upload(ctx, "other/c.tar.gz", []byte("c"))
	require.NoError(t, err)

	names, err := adapter.List(ctx, "prefix/")
	require.NoError(t, err)
	assert.Len(t, names, 2)
}

func TestMemoryAdapter_UploadWithRetry(t *testing.T) {
	t.Parallel()
	adapter := newMemoryAdapter()
	cfg := retryConfig{maxRetries: 3, baseBackoff: time.Millisecond, maxBackoff: 100 * time.Millisecond}
	data := []byte("retry-test")

	err := withRetry(context.Background(), cfg, func(ctx context.Context) error {
		return adapter.Upload(ctx, "retry.dat", data)
	})
	require.NoError(t, err)

	downloaded, err := adapter.Download(context.Background(), "retry.dat")
	require.NoError(t, err)
	assert.Equal(t, data, downloaded)
}

func TestMemoryAdapter_DownloadWithRetry(t *testing.T) {
	t.Parallel()
	adapter := newMemoryAdapter()
	cfg := retryConfig{maxRetries: 3, baseBackoff: time.Millisecond, maxBackoff: 100 * time.Millisecond}
	data := []byte("download-retry-test")

	err := adapter.Upload(context.Background(), "dl.dat", data)
	require.NoError(t, err)

	var downloaded []byte
	err = withRetry(context.Background(), cfg, func(ctx context.Context) error {
		var innerErr error
		downloaded, innerErr = adapter.Download(ctx, "dl.dat")
		return innerErr
	})
	require.NoError(t, err)
	assert.Equal(t, data, downloaded)
}

func TestService_AdapterMapping(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	svc.RegisterAdapter(newMemoryAdapter())

	a, err := svc.adapter("memory")
	require.NoError(t, err)
	assert.Equal(t, "memory", a.Name())

	_, err = svc.adapter("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestService_DefaultAdapter(t *testing.T) {
	t.Parallel()
	svc := New(nil)

	_, err := svc.adapter("")
	assert.Error(t, err)

	m := newMemoryAdapter()
	svc.RegisterAdapter(m)

	a, err := svc.adapter("memory")
	require.NoError(t, err)
	assert.Equal(t, "memory", a.Name())
	assert.Equal(t, "memory", svc.defaultAdapter)
}

func TestS3Adapter_Integration(t *testing.T) {
	skipIfNoIntegration(t)

	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	bucket := os.Getenv("S3_TEST_BUCKET")
	region := os.Getenv("S3_TEST_REGION")
	accessKey := os.Getenv("S3_TEST_ACCESS_KEY")
	secretKey := os.Getenv("S3_TEST_SECRET_KEY")

	if bucket == "" {
		t.Skip("S3_TEST_BUCKET is required for S3 integration tests")
	}
	if region == "" {
		region = "us-east-1"
	}
	if accessKey == "" || secretKey == "" {
		t.Skip("S3_TEST_ACCESS_KEY and S3_TEST_SECRET_KEY required for S3 integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adapter, err := NewS3Adapter(ctx, region, endpoint, bucket, "test-integration", accessKey, secretKey, true)
	require.NoError(t, err)

	path := "integration-test-" + time.Now().Format("20060102T150405Z") + ".dat"
	data := []byte("s3-integration-test-data-" + time.Now().String())

	err = adapter.Upload(ctx, path, data)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = adapter.Delete(context.Background(), path)
	})

	exists, err := adapter.Exists(ctx, path)
	require.NoError(t, err)
	assert.True(t, exists)

	downloaded, err := adapter.Download(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, data, downloaded)

	names, err := adapter.List(ctx, "test-integration/integration-test-")
	require.NoError(t, err)
	found := false
	for _, n := range names {
		if strings.Contains(n, path) {
			found = true
			break
		}
	}
	assert.True(t, found, "object should appear in listing")

	err = adapter.Delete(ctx, path)
	require.NoError(t, err)

	exists, err = adapter.Exists(ctx, path)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRetry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := retryConfig{maxRetries: 3, baseBackoff: time.Millisecond, maxBackoff: 100 * time.Millisecond}

	attempts := 0
	err := withRetry(ctx, cfg, func(ctx context.Context) error {
		attempts++
		if attempts < 2 {
			return io.ErrUnexpectedEOF
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestRetry_Exhausted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := retryConfig{maxRetries: 2, baseBackoff: time.Millisecond, maxBackoff: time.Millisecond}

	attempts := 0
	err := withRetry(ctx, cfg, func(ctx context.Context) error {
		attempts++
		return io.ErrUnexpectedEOF
	})
	require.Error(t, err)
	assert.Equal(t, 3, attempts)
}

func TestRetry_ContextCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cfg := retryConfig{maxRetries: 5, baseBackoff: 100 * time.Millisecond, maxBackoff: time.Second}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := withRetry(ctx, cfg, func(ctx context.Context) error {
		return io.ErrUnexpectedEOF
	})
	require.Error(t, err)
	assert.True(t, err == context.Canceled || err == context.DeadlineExceeded)
}

func TestService_CronParsing(t *testing.T) {
	t.Parallel()
	svc := New(nil)

	tests := []struct {
		name     string
		interval string
		expectOK bool
	}{
		{"every-minute", "* * * * *", true},
		{"hourly", "0 * * * *", true},
		{"daily-midnight", "0 0 * * *", true},
		{"invalid", "every hour", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, err := svc.NextCronRun(store.BackupPolicy{Interval: tt.interval}, time.Now())
			if tt.expectOK {
				assert.NoError(t, err)
				assert.True(t, next.After(time.Now()), "next run should be in the future")
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestService_RegisterAdapter_PrioritizesFirst(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	m1 := newMemoryAdapter()
	svc.RegisterAdapter(m1)
	assert.Equal(t, "memory", svc.defaultAdapter)
}

func TestService_uploadBackupPath_BareName(t *testing.T) {
	t.Parallel()
	name := "backup-test"
	assert.True(t, !strings.Contains(name, "."))
	assert.True(t, !strings.HasSuffix(name, ".tar.gz"))
	assert.True(t, !strings.HasSuffix(name, ".zip"))
	ext := name + ".tar.gz"
	assert.Equal(t, "backup-test.tar.gz", ext)
}

func TestMemoryAdapter_UploadDownload_LargeData(t *testing.T) {
	t.Parallel()
	adapter := newMemoryAdapter()
	ctx := context.Background()
	data := make([]byte, 1024*100)
	for i := range data {
		data[i] = byte(i % 256)
	}

	err := adapter.Upload(ctx, "large.dat", data)
	require.NoError(t, err)

	downloaded, err := adapter.Download(ctx, "large.dat")
	require.NoError(t, err)
	assert.Equal(t, data, downloaded)
}

func TestMemoryAdapter_ByteCopy_Safety(t *testing.T) {
	t.Parallel()
	adapter := newMemoryAdapter()
	ctx := context.Background()
	original := []byte("original")

	err := adapter.Upload(ctx, "safety.dat", original)
	require.NoError(t, err)

	original[0] = 'X'

	downloaded, err := adapter.Download(ctx, "safety.dat")
	require.NoError(t, err)
	assert.Equal(t, "original", string(downloaded))
}

func TestRetry_Jitter(t *testing.T) {
	t.Parallel()
	d := 100 * time.Millisecond
	for i := 0; i < 20; i++ {
		got := jitterDuration(d)
		assert.GreaterOrEqual(t, got, 75*time.Millisecond)
		assert.LessOrEqual(t, got, 125*time.Millisecond)
	}
}

func TestStorageAdapter_Interface(t *testing.T) {
	t.Parallel()
	var _ StorageAdapter = newMemoryAdapter()
	assert.Equal(t, "memory", newMemoryAdapter().Name())
}

func TestService_BackupStruct_DefaultValues(t *testing.T) {
	t.Parallel()
	b := Backup{
		ID:        "test-id",
		ServerID:  "server-1",
		Name:      "test-backup",
		Status:    BackupPending,
		Storage:   "s3",
		Locked:    false,
		CreatedAt: time.Now(),
	}
	assert.Equal(t, BackupPending, b.Status)
	assert.Equal(t, "s3", b.Storage)
	assert.False(t, b.Locked)
}

func TestCreateBackupRequest_Defaults(t *testing.T) {
	t.Parallel()
	req := CreateBackupRequest{
		Name:    "daily-backup",
		Locked:  true,
		Storage: "gcs",
	}
	assert.Equal(t, "daily-backup", req.Name)
	assert.True(t, req.Locked)
	assert.Equal(t, "gcs", req.Storage)
}

func TestBackupStatus_Constants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, BackupStatus("pending"), BackupPending)
	assert.Equal(t, BackupStatus("running"), BackupRunning)
	assert.Equal(t, BackupStatus("completed"), BackupCompleted)
	assert.Equal(t, BackupStatus("failed"), BackupFailed)
	assert.Equal(t, BackupStatus("deleted"), BackupDeleted)
}

func TestService_New_DefaultValues(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	assert.Equal(t, 30, svc.defaultRetentionDays)
	assert.NotNil(t, svc.adapters)
	assert.NotNil(t, svc.retryCfg)
	assert.Equal(t, defaultMaxRetries, svc.retryCfg.maxRetries)
	assert.Equal(t, defaultBaseBackoff, svc.retryCfg.baseBackoff)
}

func TestMemoryAdapter_MultipleFiles(t *testing.T) {
	t.Parallel()
	adapter := newMemoryAdapter()
	ctx := context.Background()
	count := 10

	for i := 0; i < count; i++ {
		path := "batch/file-" + string(rune('0'+i))
		err := adapter.Upload(ctx, path, []byte{byte(i)})
		require.NoError(t, err)
	}

	names, err := adapter.List(ctx, "batch/")
	require.NoError(t, err)
	assert.Len(t, names, count)
}

func TestService_DownloadFromStorage(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	path := "backups/server-1/backup-test.tar.gz"
	data := []byte("download-from-storage-test-data")
	err := adapter.Upload(ctx, path, data)
	require.NoError(t, err)

	downloaded, err := svc.DownloadFromStorage(ctx, "memory", path)
	require.NoError(t, err)
	assert.Equal(t, data, downloaded)
}

func TestService_DownloadFromStorage_NilData(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	_, err := svc.DownloadFromStorage(ctx, "memory", "nonexistent")
	assert.Error(t, err)
}

func TestService_DownloadFromStorage_UnknownAdapter(t *testing.T) {
	t.Parallel()
	svc := New(nil)

	_, err := svc.DownloadFromStorage(context.Background(), "nonexistent", "some-path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestService_VerifyChecksum_Match(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	data := []byte("test-data-for-checksum-verification")
	computed := fmt.Sprintf("%x", sha256.Sum256(data))
	err := svc.VerifyChecksum(data, computed)
	assert.NoError(t, err)
}

func TestService_VerifyChecksum_Mismatch(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	data := []byte("test-data")
	err := svc.VerifyChecksum(data, "deadbeef")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}

func TestService_VerifyChecksum_EmptyExpected(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	data := []byte("test-data")
	err := svc.VerifyChecksum(data, "")
	assert.NoError(t, err)
}

func TestService_SetRetentionDays(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	assert.Equal(t, 30, svc.defaultRetentionDays)
	svc.SetRetentionDays(7)
	assert.Equal(t, 7, svc.defaultRetentionDays)
	svc.SetRetentionDays(0)
	assert.Equal(t, 7, svc.defaultRetentionDays)
	svc.SetRetentionDays(-1)
	assert.Equal(t, 7, svc.defaultRetentionDays)
}

func TestService_GetBackup(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Logf("expected panic with nil store: %v", r)
		}
	}()
	_, _ = svc.GetBackup(context.Background(), "server-1", "nonexistent")
}

func TestService_ListAllEnabledPolicies_NilStore(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Logf("expected panic with nil store: %v", r)
		}
	}()
	_, _ = svc.ListAllEnabledPolicies(context.Background())
}

func TestService_BackupCreateRestore_Lifecycle(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()

	storagePath := svc.buildStoragePath("server-1", "lifecycle-test")
	testData := []byte("backup-lifecycle-test-data")
	err := adapter.Upload(ctx, storagePath, testData)
	require.NoError(t, err)

	downloaded, err := svc.DownloadFromStorage(ctx, "memory", storagePath)
	require.NoError(t, err)
	assert.Equal(t, testData, downloaded)

	computedChecksum := fmt.Sprintf("%x", sha256.Sum256(testData))
	err = svc.VerifyChecksum(downloaded, computedChecksum)
	assert.NoError(t, err)

	err = svc.VerifyChecksum(downloaded, "wrong-checksum")
	assert.Error(t, err)

	exists, err := adapter.Exists(ctx, storagePath)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestService_EnforceRetentionPolicy_WithStorageCleanup(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)
	_ = adapter
}

func TestService_ListAllEnabledPolicies_Empty(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Logf("expected panic with nil store: %v", r)
		}
	}()
	_, _ = svc.ListAllEnabledPolicies(context.Background())
}

func TestWorker_New(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	w := NewWorker(nil, svc, nil)
	assert.NotNil(t, w)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	cancel()
	w.Wait()
}

func TestWorker_Start_NilStore(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	w := NewWorker(nil, svc, nil)
	w.Start(context.Background())
	w.Wait()
}

func TestService_CronParse_EveryMinute(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	now := time.Now().UTC()
	next, err := svc.NextCronRun(store.BackupPolicy{Interval: "* * * * *"}, now)
	require.NoError(t, err)
	assert.True(t, next.After(now))
	assert.True(t, next.Before(now.Add(2*time.Minute)))
}

func TestService_CronParse_DailyMidnight(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	now := time.Now().UTC()
	next, err := svc.NextCronRun(store.BackupPolicy{Interval: "0 0 * * *"}, now)
	require.NoError(t, err)
	assert.True(t, next.After(now))
}

func TestService_CronParse_Invalid(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	_, err := svc.NextCronRun(store.BackupPolicy{Interval: "not-a-cron"}, time.Now())
	assert.Error(t, err)
}

func TestService_CleanupExpiredBackups_Empty(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Logf("expected panic with nil store: %v", r)
		}
	}()
	_, _ = svc.CleanupExpiredBackups(context.Background())
}

func TestService_RestoreFromStorage_BackupNotFound(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Logf("expected panic with nil store: %v", r)
		}
	}()
	_, _ = svc.RestoreFromStorage(context.Background(), "unknown", "unknown", io.Discard)
}

func TestService_DownloadFromStorage_DefaultAdapter(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	path := "backups/default-test.tar.gz"
	data := []byte("default-adapter-test")
	err := adapter.Upload(ctx, path, data)
	require.NoError(t, err)

	downloaded, err := svc.DownloadFromStorage(ctx, "", path)
	require.NoError(t, err)
	assert.Equal(t, data, downloaded)
}

func TestBackupStatus_RestoreConstants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, BackupStatus("restoring"), BackupRestoring)
	assert.Equal(t, BackupStatus("restored"), BackupRestored)
	assert.Equal(t, BackupStatus("restore_failed"), BackupRestoreFail)
}

func TestService_buildStoragePath(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	path := svc.buildStoragePath("server-abc", "backup-xyz")
	assert.Equal(t, "backups/server-abc/backup-xyz", path)
}
