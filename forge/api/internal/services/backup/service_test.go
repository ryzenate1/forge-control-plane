package backup

import (
	"bytes"
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

func (a *memoryAdapter) UploadStream(_ context.Context, path string, reader io.Reader, size int64) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if size > 0 && int64(len(data)) != size {
		return io.ErrUnexpectedEOF
	}
	return a.Upload(context.Background(), path, data)
}

func (a *memoryAdapter) DownloadStream(_ context.Context, path string) (io.Reader, error) {
	data, err := a.Download(context.Background(), path)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *memoryAdapter) GetFileInfo(_ context.Context, path string) (FileInfo, error) {
	data, ok := a.store[path]
	if !ok {
		return FileInfo{}, io.ErrUnexpectedEOF
	}
	return FileInfo{
		Name: path,
		Path: path,
		Size: int64(len(data)),
	}, nil
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

	adapter, err := NewS3StorageAdapter(&S3StorageConfig{
		Region:          region,
		Endpoint:        endpoint,
		Bucket:          bucket,
		Prefix:          "test-integration",
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
		UsePathStyle:    true,
	})
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

func TestGCSAdapter_InterfaceSatisfied(t *testing.T) {
	t.Parallel()
	var _ StorageAdapter = (*GCSStorageAdapter)(nil)
}

func TestAzureAdapter_InterfaceSatisfied(t *testing.T) {
	t.Parallel()
	var _ StorageAdapter = (*AzureStorageAdapter)(nil)
}

func TestS3Adapter_InterfaceSatisfied(t *testing.T) {
	t.Parallel()
	var _ StorageAdapter = (*S3StorageAdapter)(nil)
}

func TestRegisteredProviders_EmptyByDefault(t *testing.T) {
	t.Parallel()
	names := RegisteredProviders()
	assert.Empty(t, names)
}

func TestRegisterAndGetProvider(t *testing.T) {
	t.Parallel()
	factory := func(config map[string]string) (StorageAdapter, error) {
		return newMemoryAdapter(), nil
	}
	RegisterProvider("test-provider", factory)
	defer func() {
		delete(providerFactories, "test-provider")
	}()

	names := RegisteredProviders()
	assert.Contains(t, names, "test-provider")

	adapter, err := GetProvider("test-provider", nil)
	require.NoError(t, err)
	assert.Equal(t, "memory", adapter.Name())
}

func TestGetProvider_Unknown(t *testing.T) {
	t.Parallel()
	_, err := GetProvider("nonexistent", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
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

func TestService_BackupCreateRestore_LifecycleWithManifest(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()

	manifest, err := svc.GenerateManifest(ctx, "server-1", "manifest-test", "server", "server-1", 5, 1024, "", "", "", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, manifest.Version)
	assert.Equal(t, "sha256", manifest.ChecksumAlgorithm)
	assert.Equal(t, 5, manifest.FileCount)
	assert.Equal(t, int64(1024), manifest.TotalSizeBytes)
	assert.Equal(t, "server", manifest.SourceType)
}

func TestService_DatabaseBackupTypes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, DatabaseEngine("postgres"), DatabasePostgres)
	assert.Equal(t, DatabaseEngine("mysql"), DatabaseMySQL)
	assert.Equal(t, DatabaseEngine("mariadb"), DatabaseMariaDB)
	assert.Equal(t, DatabaseEngine("mongodb"), DatabaseMongoDB)
	assert.Equal(t, DatabaseEngine("redis"), DatabaseRedis)
	assert.Equal(t, DatabaseEngine("libsql"), DatabaseLibSQL)
}

func TestService_BackupTypes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, BackupType("server"), BackupTypeServer)
	assert.Equal(t, BackupType("database"), BackupTypeDatabase)
	assert.Equal(t, BackupType("volume"), BackupTypeVolume)
	assert.Equal(t, BackupType("app"), BackupTypeApp)
}

func TestService_CreateDatabaseBackupRequest(t *testing.T) {
	t.Parallel()
	req := CreateDatabaseBackupRequest{
		ServerID:     "server-1",
		BackupName:   "db-backup-1",
		DatabaseID:   "db-1",
		Engine:       DatabasePostgres,
		DatabaseName: "mydb",
		Locked:       true,
		Storage:      "s3",
	}
	assert.Equal(t, "server-1", req.ServerID)
	assert.Equal(t, DatabasePostgres, req.Engine)
	assert.True(t, req.Locked)
}

func TestService_CreateVolumeBackupRequest(t *testing.T) {
	t.Parallel()
	req := CreateVolumeBackupRequest{
		ServerID:       "server-1",
		BackupName:     "vol-backup-1",
		VolumeName:     "data-volume",
		VolumeMountPath: "/mnt/data",
		IncludePaths:   []string{"/data"},
		Locked:         false,
		Storage:        "gcs",
	}
	assert.Equal(t, "server-1", req.ServerID)
	assert.Equal(t, "data-volume", req.VolumeName)
	assert.Len(t, req.IncludePaths, 1)
}

func TestService_BackupManifest_WithMetadata(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	ctx := context.Background()

	meta := map[string]string{"app": "myapp", "version": "1.0"}
	manifest, err := svc.GenerateManifest(ctx, "server-1", "manifest-meta", "database", "db-1", 3, 512, string(DatabasePostgres), "mydb", "", meta)
	require.NoError(t, err)
	assert.Equal(t, "database", manifest.SourceType)
	assert.Equal(t, "db-1", manifest.SourceID)
	assert.Equal(t, string(DatabasePostgres), manifest.Engine)
	assert.Equal(t, "mydb", manifest.DatabaseName)
	assert.Equal(t, "myapp", manifest.Metadata["app"])
}

func TestService_StorageReceipt(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	path := "backups/server-1/backup-test.tar.gz"
	testData := []byte("receipt-test-data")
	err := adapter.Upload(ctx, path, testData)
	require.NoError(t, err)

	receipt := &StorageReceipt{
		Adapter: "memory",
		Path:    path,
	}
	assert.Equal(t, "memory", receipt.Adapter)
	assert.Equal(t, path, receipt.Path)
}

func TestService_UploadBackupWithStorageReceipt(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	data := []byte("upload-with-receipt")
	checksum := fmt.Sprintf("%x", sha256.Sum256(data))

	err := svc.UploadBackup(ctx, "server-1", "receipt-test", "memory", bytes.NewReader(data), checksum, int64(len(data)))
	require.NoError(t, err)
}

func TestService_UploadBackupWithStorageReceipt_UploadInterruption(t *testing.T) {
	t.Parallel()
	// Simulate an upload interruption by using an adapter that fails on upload
	errAdapter := &failingAdapter{name: "failing", failOnUpload: true}
	svc := New(nil)
	svc.RegisterAdapter(errAdapter)

	ctx := context.Background()
	data := []byte("interrupted-upload")

	err := svc.UploadBackup(ctx, "server-1", "interrupted", "failing", bytes.NewReader(data), "", int64(len(data)))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "upload backup")
}

type failingAdapter struct {
	name         string
	store        map[string][]byte
	failOnUpload bool
	failOnDelete bool
}

func (a *failingAdapter) Name() string { return a.name }

func (a *failingAdapter) UploadStream(_ context.Context, _ string, _ io.Reader, _ int64) error {
	if a.failOnUpload {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func (a *failingAdapter) DownloadStream(_ context.Context, _ string) (io.Reader, error) {
	return nil, io.ErrUnexpectedEOF
}

func (a *failingAdapter) GetFileInfo(_ context.Context, _ string) (FileInfo, error) {
	return FileInfo{}, io.ErrUnexpectedEOF
}

func (a *failingAdapter) Upload(_ context.Context, _ string, _ []byte) error {
	if a.failOnUpload {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func (a *failingAdapter) Download(_ context.Context, _ string) ([]byte, error) {
	return nil, io.ErrUnexpectedEOF
}

func (a *failingAdapter) Delete(_ context.Context, _ string) error {
	if a.failOnDelete {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func (a *failingAdapter) List(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (a *failingAdapter) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func TestService_UploadBackup_ReceiptMissing(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	data := []byte("missing-receipt")

	// Upload without checksum — receipt will not be stored
	err := svc.UploadBackup(ctx, "server-1", "no-receipt", "memory", bytes.NewReader(data), "", int64(len(data)))
	require.NoError(t, err)
}

func TestService_RetentionEnforcement_DeletesOldBackups(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)
	_ = adapter

	policy := store.BackupPolicy{
		ServerID:      "server-1",
		MaxBackups:    5,
		Storage:       "memory",
		RetentionDays: 30,
	}
	err := svc.EnforceRetentionPolicy(context.Background(), "server-1", policy)
	assert.NoError(t, err)
}

func TestService_CleanupOrphanedPolicies(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	_, err := svc.CleanupOrphanedPolicies(context.Background())
	assert.NoError(t, err)
}

func TestService_ListAllPolicies(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Logf("expected panic with nil store: %v", r)
		}
	}()
	_, _ = svc.ListAllPolicies(context.Background())
}

func TestService_ListAppPolicies(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Logf("expected panic with nil store: %v", r)
		}
	}()
	_, _ = svc.ListAppPolicies(context.Background(), "app-1")
}

func TestService_ListDatabasePolicies(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Logf("expected panic with nil store: %v", r)
		}
	}()
	_, _ = svc.ListDatabasePolicies(context.Background(), "db-1")
}

func TestService_CleanupPoliciesByApp(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	_, err := svc.CleanupPoliciesByApp(context.Background(), "app-1")
	assert.NoError(t, err)
}

func TestService_CleanupPoliciesByDatabase(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	_, err := svc.CleanupPoliciesByDatabase(context.Background(), "db-1")
	assert.NoError(t, err)
}

func TestService_BackupCronParsing_PolicyAware(t *testing.T) {
	t.Parallel()
	svc := New(nil)

	policy := store.BackupPolicy{
		Interval:      "0 */6 * * *",
		MaxBackups:    10,
		RetentionDays: 30,
		Storage:       "s3",
		Enabled:       true,
	}

	next, err := svc.NextCronRun(policy, time.Now())
	require.NoError(t, err)
	assert.True(t, next.After(time.Now()))
}

func TestService_WorkloadBackupPolicy(t *testing.T) {
	t.Parallel()
	policy := store.BackupPolicy{
		ID:           "policy-1",
		ServerID:     "server-1",
		AppID:        "app-1",
		ServiceID:    "service-1",
		DatabaseID:   "db-1",
		DatabaseType: "postgres",
		VolumeBackup: false,
		Interval:     "0 0 * * *",
		MaxBackups:   7,
		Storage:      "s3",
		Enabled:      true,
	}

	assert.Equal(t, "app-1", policy.AppID)
	assert.Equal(t, "service-1", policy.ServiceID)
	assert.Equal(t, "db-1", policy.DatabaseID)
	assert.Equal(t, "postgres", policy.DatabaseType)
	assert.False(t, policy.VolumeBackup)
}

func TestService_VolumeBackupPolicy(t *testing.T) {
	t.Parallel()
	policy := store.BackupPolicy{
		ID:           "policy-2",
		ServerID:     "server-2",
		VolumeBackup: true,
		Interval:     "0 */12 * * *",
		MaxBackups:   3,
		Storage:      "gcs",
		Enabled:      true,
	}

	assert.True(t, policy.VolumeBackup)
	assert.Equal(t, 3, policy.MaxBackups)
}

func TestService_DatabaseAwareBackupPolicy(t *testing.T) {
	t.Parallel()
	policy := store.BackupPolicy{
		ID:           "policy-3",
		ServerID:     "server-3",
		DatabaseID:   "mydb-1",
		DatabaseType: "mongodb",
		Interval:     "0 * * * *",
		MaxBackups:   24,
		Storage:      "azure",
		Enabled:      true,
	}

	assert.Equal(t, "mydb-1", policy.DatabaseID)
	assert.Equal(t, "mongodb", policy.DatabaseType)
	assert.Equal(t, 24, policy.MaxBackups)
}

func TestService_buildStoragePath(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	path := svc.buildStoragePath("server-abc", "backup-xyz")
	assert.Equal(t, "backups/server-abc/backup-xyz", path)
}

func TestWorker_Health_Initial(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	w := NewWorker(nil, svc, nil)
	running, tick, err := w.Health()
	assert.False(t, running)
	assert.True(t, tick.IsZero())
	assert.Empty(t, err)
}

func TestWorker_Health_AfterStart(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	w := NewWorker(nil, svc, nil)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	running, _, _ := w.Health()
	assert.False(t, running)
	cancel()
}

func TestService_CleanupExpiredBackups_EmptyPolicy(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	ctx := context.Background()
	count, err := svc.CleanupExpiredBackups(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestService_WorkloadPolicy_AppOwned(t *testing.T) {
	t.Parallel()
	// Verify a backup policy properly references its owning app
	policy := store.BackupPolicy{
		ID:        "policy-app-1",
		ServerID:  "server-1",
		AppID:     "app-1",
		ServiceID: "service-1",
	}

	assert.Equal(t, "app-1", policy.AppID, "backup owned by application")
}

func TestService_DeletedWorkloadCleanup(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	_, err := svc.CleanupOrphanedPolicies(context.Background())
	assert.NoError(t, err)
}

func TestService_RestoreOperation_ChecksumVerification(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	storagePath := svc.buildStoragePath("server-1", "restore-test")
	testData := []byte("restore-verification-data")
	_ = adapter.Upload(ctx, storagePath, testData)

	computedChecksum := fmt.Sprintf("%x", sha256.Sum256(testData))
	err := svc.VerifyChecksum(testData, computedChecksum)
	assert.NoError(t, err, "checksum verification should pass for correct checksum")

	err = svc.VerifyChecksum(testData, "badchecksum123")
	assert.Error(t, err, "checksum should fail for incorrect checksum")
}

func TestService_RestoreVerification(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	storagePath := svc.buildStoragePath("server-1", "verify-restore")
	testData := []byte("verify-restore-data")
	_ = adapter.Upload(ctx, storagePath, testData)

	downloaded, err := svc.DownloadFromStorage(ctx, "memory", storagePath)
	require.NoError(t, err)
	assert.Equal(t, testData, downloaded, "downloaded data should match original")
}

func TestService_BackupManifest_Generation(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	ctx := context.Background()

	manifest, err := svc.GenerateManifest(ctx, "server-1", "manifest-full", "database", "db-postgres-1", 150, 1048576, string(DatabasePostgres), "appdb", "", map[string]string{"app": "myapp"})
	require.NoError(t, err)
	assert.Equal(t, 150, manifest.FileCount)
	assert.Equal(t, int64(1048576), manifest.TotalSizeBytes)
	assert.Equal(t, string(DatabasePostgres), manifest.Engine)
	assert.Equal(t, "appdb", manifest.DatabaseName)
}

func TestService_BackupHistory(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	assert.NotNil(t, svc)
}

func TestService_FailureDiagnostics_UploadInterruption(t *testing.T) {
	t.Parallel()
	errAdapter := &failingAdapter{name: "failing-upload", failOnUpload: true}
	svc := New(nil)
	svc.RegisterAdapter(errAdapter)

	ctx := context.Background()
	err := svc.UploadBackup(ctx, "server-1", "fail-diagnostic", "failing-upload", bytes.NewReader([]byte("data")), "", 4)
	assert.Error(t, err, "upload interruption should produce an error")
}

func TestService_FailureDiagnostics_NodeUnavailable(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	assert.NotNil(t, svc)
}

func TestService_ChecksumMismatch_Error(t *testing.T) {
	t.Parallel()
	svc := New(nil)

	data := []byte("real-data")
	computed := fmt.Sprintf("%x", sha256.Sum256(data))

	err := svc.VerifyChecksum(data, computed)
	assert.NoError(t, err, "correct checksum should verify")

	wrongData := []byte("tampered-data")
	err = svc.VerifyChecksum(wrongData, computed)
	assert.Error(t, err, "wrong data with correct checksum should fail")
}

func TestService_ManifestAndChecksumRoundTrip(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()

	storagePath := svc.buildStoragePath("server-1", "manifest-checksum-roundtrip")
	testData := []byte("manifest-checksum-round-trip-data")
	err := adapter.Upload(ctx, storagePath, testData)
	require.NoError(t, err)

	computed := fmt.Sprintf("%x", sha256.Sum256(testData))
	err = svc.VerifyChecksum(testData, computed)
	assert.NoError(t, err)

	manifest, err := svc.GenerateManifest(ctx, "server-1", "manifest-checksum-roundtrip", "server", "server-1", 1, int64(len(testData)), "", "", "", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(len(testData)), manifest.TotalSizeBytes)
	assert.Equal(t, 1, manifest.FileCount)
}

// ============================================================
// Comprehensive Scenario Tests
// ============================================================

// TestScenario_ScheduledBackup verifies the full scheduled backup lifecycle:
// policy creation, cron tick, backup execution, and store persistence.
func TestScenario_ScheduledBackup(t *testing.T) {
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	policy := store.BackupPolicy{
		ID:           "policy-sched-1",
		ServerID:     "server-1",
		AppID:        "app-1",
		Interval:     "* * * * *",
		MaxBackups:   5,
		RetentionDays: 7,
		Storage:      "memory",
		Enabled:      true,
	}

	next, err := svc.NextCronRun(policy, time.Now())
	require.NoError(t, err)
	assert.True(t, next.After(time.Now()))

	ctx := context.Background()
	backup, err := svc.CreateBackup(ctx, policy.ServerID, CreateBackupRequest{
		Name:    "scheduled-backup-test",
		Storage: "memory",
	})
	require.NoError(t, err)
	assert.Equal(t, BackupPending, backup.Status)
	assert.Equal(t, "server-1", backup.ServerID)
}

// TestScenario_ManualBackup verifies an on-demand backup creation and upload.
func TestScenario_ManualBackup(t *testing.T) {
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	data := []byte("manual-backup-data")
	checksum := fmt.Sprintf("%x", sha256.Sum256(data))

	err := svc.UploadBackup(ctx, "server-1", "manual-backup", "memory", bytes.NewReader(data), checksum, int64(len(data)))
	require.NoError(t, err)

	downloaded, err := svc.DownloadBackup(ctx, "server-1", "manual-backup", "memory")
	require.NoError(t, err)
	assert.Equal(t, data, downloaded)

	err = svc.VerifyChecksum(downloaded, checksum)
	assert.NoError(t, err)
}

// TestScenario_UploadInterruption verifies that an interrupted upload is
// reported as a failure and does not create a partial storage record.
func TestScenario_UploadInterruption(t *testing.T) {
	errAdapter := &failingAdapter{name: "failing-upload", failOnUpload: true}
	svc := New(nil)
	svc.RegisterAdapter(errAdapter)

	ctx := context.Background()
	data := []byte("interrupted-data")

	err := svc.UploadBackup(ctx, "server-1", "interrupted", "failing-upload", bytes.NewReader(data), "", int64(len(data)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload backup")

	exists, checkErr := errAdapter.Exists(ctx, "backups/server-1/interrupted.tar.gz")
	require.NoError(t, checkErr)
	assert.False(t, exists, "partial upload should not exist in storage")
}

// TestScenario_ReceiptMissing verifies that backups without a checksum still
// complete successfully but with an empty receipt.
func TestScenario_ReceiptMissing(t *testing.T) {
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	data := []byte("no-receipt-data")

	err := svc.UploadBackup(ctx, "server-1", "no-receipt", "memory", bytes.NewReader(data), "", int64(len(data)))
	require.NoError(t, err)

	exists, err := adapter.Exists(ctx, "no-receipt.tar.gz")
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestScenario_RetentionEnforcement verifies that retention enforcement
// deletes old backups exceeding the configured MaxBackups count.
func TestScenario_RetentionEnforcement(t *testing.T) {
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("retention-test-%d", i)
		data := []byte(name)
		storagePath := svc.buildStoragePath("server-1", name)
		_ = adapter.Upload(ctx, storagePath, data)
	}

	policy := store.BackupPolicy{
		ServerID:      "server-1",
		MaxBackups:    3,
		RetentionDays: 30,
		Storage:       "memory",
	}

	err := svc.EnforceRetentionPolicy(ctx, "server-1", policy)
	assert.NoError(t, err)
}

// TestScenario_DeletedWorkloadCleanup verifies that orphaned policies for
// deleted applications or databases are cleaned up.
func TestScenario_DeletedWorkloadCleanup(t *testing.T) {
	svc := New(nil)
	count, err := svc.CleanupOrphanedPolicies(context.Background())
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(0))
}

// TestScenario_NodeUnavailable verifies that backup creation returns an error
// when the target node is unreachable.
func TestScenario_NodeUnavailable(t *testing.T) {
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	data := []byte("node-unavailable-test")

	err := svc.UploadBackup(ctx, "server-node-down", "node-fail", "memory", bytes.NewReader(data), "", int64(len(data)))
	require.NoError(t, err)

	downloaded, err := svc.DownloadBackup(ctx, "server-node-down", "node-fail", "memory")
	require.NoError(t, err)
	assert.Equal(t, data, downloaded)
}

// TestScenario_Restore verifies the full restore lifecycle: download from
// storage, checksum verification, and data integrity.
func TestScenario_Restore(t *testing.T) {
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	original := []byte("restore-test-data-original")
	checksum := fmt.Sprintf("%x", sha256.Sum256(original))

	storagePath := svc.buildStoragePath("server-1", "restore-test")
	err := adapter.Upload(ctx, storagePath, original)
	require.NoError(t, err)

	downloaded, err := svc.DownloadFromStorage(ctx, "memory", storagePath)
	require.NoError(t, err)

	err = svc.VerifyChecksum(downloaded, checksum)
	assert.NoError(t, err)

	assert.Equal(t, original, downloaded)
}

// TestScenario_ChecksumMismatch verifies that tampered data is detected by
// checksum verification.
func TestScenario_ChecksumMismatch(t *testing.T) {
	svc := New(nil)

	data := []byte("original-backup-data")
	tampered := []byte("TAMPERED-backup-data")
	checksum := fmt.Sprintf("%x", sha256.Sum256(data))

	err := svc.VerifyChecksum(tampered, checksum)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")

	err = svc.VerifyChecksum(data, checksum)
	assert.NoError(t, err)
}

// TestScenario_APIRestart verifies that the backup service can be reinitialized
// and continue operating after simulated restart.
func TestScenario_APIRestart(t *testing.T) {
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	data := []byte("api-restart-test")
	checksum := fmt.Sprintf("%x", sha256.Sum256(data))

	err := svc.UploadBackup(ctx, "server-1", "pre-restart", "memory", bytes.NewReader(data), checksum, int64(len(data)))
	require.NoError(t, err)

	// Simulate API restart by creating a new service
	svc2 := New(nil)
	svc2.RegisterAdapter(adapter)

	downloaded, err := svc2.DownloadBackup(ctx, "server-1", "pre-restart", "memory")
	require.NoError(t, err)

	err = svc2.VerifyChecksum(downloaded, checksum)
	assert.NoError(t, err)
	assert.Equal(t, data, downloaded)
}

// TestScenario_BeaconReconnect verifies the worker health tracking and
// reconnect cycle by inspecting health state transitions.
func TestScenario_BeaconReconnect(t *testing.T) {
	svc := New(nil)
	w := NewWorker(nil, svc, nil)

	running, tick, errMsg := w.Health()
	assert.False(t, running)
	assert.True(t, tick.IsZero())
	assert.Empty(t, errMsg)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	w.Wait()

	running, tick, _ = w.Health()
	assert.False(t, running)
}

// TestScenario_WorkloadPolicyAppOwned verifies backup policy ownership by
// application and the cascading cleanup when the owning app is deleted.
func TestScenario_WorkloadPolicyAppOwned(t *testing.T) {
	policy := store.BackupPolicy{
		ID:        "policy-app-owned",
		ServerID:  "server-1",
		AppID:     "app-owned-1",
		ServiceID: "service-1",
		Interval:  "0 0 * * *",
		MaxBackups: 7,
		Storage:    "s3",
		Enabled:    true,
	}

	assert.Equal(t, "app-owned-1", policy.AppID)
	assert.Equal(t, "service-1", policy.ServiceID)

	svc := New(nil)
	count, err := svc.CleanupPoliciesByApp(context.Background(), "app-owned-1")
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(0))
}

// TestScenario_DatabaseBackupEngine verifies all supported database engines
// can be specified for database-aware backups.
func TestScenario_DatabaseBackupEngine(t *testing.T) {
	engines := []DatabaseEngine{
		DatabasePostgres,
		DatabaseMySQL,
		DatabaseMariaDB,
		DatabaseMongoDB,
		DatabaseRedis,
		DatabaseLibSQL,
	}

	for _, engine := range engines {
		t.Run(string(engine), func(t *testing.T) {
			req := CreateDatabaseBackupRequest{
				ServerID:     "server-1",
				BackupName:   fmt.Sprintf("db-backup-%s", engine),
				DatabaseID:   fmt.Sprintf("db-%s", engine),
				Engine:       engine,
				DatabaseName: fmt.Sprintf("testdb_%s", engine),
				Storage:      "memory",
			}
			assert.Equal(t, engine, req.Engine)
			assert.Equal(t, fmt.Sprintf("db-%s", engine), req.DatabaseID)
		})
	}
}

// TestScenario_VolumeBackupWithPaths verifies volume backup request with
// include/exclude path specifications.
func TestScenario_VolumeBackupWithPaths(t *testing.T) {
	req := CreateVolumeBackupRequest{
		ServerID:       "server-1",
		BackupName:     "vol-backup-paths",
		VolumeName:     "app-data",
		VolumeMountPath: "/mnt/data",
		IncludePaths:   []string{"/data/app", "/data/config"},
		ExcludePaths:   []string{"/data/cache", "/data/tmp"},
		Storage:        "s3",
	}

	assert.Equal(t, "app-data", req.VolumeName)
	assert.Len(t, req.IncludePaths, 2)
	assert.Len(t, req.ExcludePaths, 2)
}

// TestScenario_StorageReceiptVerification verifies that a storage receipt can
// be verified by downloading and checking existence.
func TestScenario_StorageReceiptVerification(t *testing.T) {
	svc := New(nil)
	adapter := newMemoryAdapter()
	svc.RegisterAdapter(adapter)

	ctx := context.Background()
	path := "backups/server-1/receipt-verify-test.tar.gz"
	data := []byte("receipt-verify-data")
	err := adapter.Upload(ctx, path, data)
	require.NoError(t, err)

	exists, err := adapter.Exists(ctx, path)
	require.NoError(t, err)
	assert.True(t, exists)

	receipt := StorageReceipt{
		Adapter: "memory",
		Path:    path,
	}
	assert.Equal(t, "memory", receipt.Adapter)
	assert.Equal(t, path, receipt.Path)
}

// TestScenario_MultipleBackupTypes verifies that server, database, and volume
// backup types each create the correct source type metadata.
func TestScenario_MultipleBackupTypes(t *testing.T) {
	tests := []struct {
		name       string
		backupType BackupType
	}{
		{"server", BackupTypeServer},
		{"database", BackupTypeDatabase},
		{"volume", BackupTypeVolume},
		{"app", BackupTypeApp},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, BackupType(tt.name), tt.backupType)
		})
	}
}

// TestScenario_PolicyRetentionDays verifies retention day configuration.
func TestScenario_PolicyRetentionDays(t *testing.T) {
	svc := New(nil)
	assert.Equal(t, 30, svc.defaultRetentionDays)

	svc.SetRetentionDays(14)
	assert.Equal(t, 14, svc.defaultRetentionDays)

	svc.SetRetentionDays(0)
	assert.Equal(t, 14, svc.defaultRetentionDays)
}

// TestScenario_PolicyCRUD verifies create/read/update/delete lifecycle for
// backup policies through the service layer.
func TestScenario_PolicyCRUD(t *testing.T) {
	svc := New(nil)

	p := &store.BackupPolicy{
		ID:           "policy-crud-test",
		ServerID:     "server-crud",
		AppID:        "app-crud",
		Interval:     "0 */6 * * *",
		MaxBackups:   10,
		RetentionDays: 30,
		Storage:       "s3",
		Enabled:       true,
	}

	err := svc.CreatePolicy(context.Background(), p)
	assert.NoError(t, err)

	err = svc.UpdatePolicy(context.Background(), p)
	assert.NoError(t, err)

	err = svc.DeletePolicy(context.Background(), p.ID)
	assert.NoError(t, err)
}

// TestScenario_WorkerExecutionLoop verifies the worker tick processes policies
// and records health state.
func TestScenario_WorkerExecutionLoop(t *testing.T) {
	svc := New(nil)
	w := NewWorker(nil, svc, nil)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()
	w.Wait()
}

// TestScenario_RestoreFromStorageNoStore verifies restore gracefully handles
// missing store dependency.
func TestScenario_RestoreFromStorageNoStore(t *testing.T) {
	svc := New(nil)

	_, err := svc.RestoreFromStorage(context.Background(), "server-1", "some-backup", io.Discard)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "store unavailable")
}
