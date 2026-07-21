package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"gamepanel/forge/internal/store"
)

type BackupStatus string

const (
	BackupPending     BackupStatus = "pending"
	BackupRunning     BackupStatus = "running"
	BackupCompleted   BackupStatus = "completed"
	BackupFailed      BackupStatus = "failed"
	BackupDeleted     BackupStatus = "deleted"
	BackupRestoring   BackupStatus = "restoring"
	BackupRestored    BackupStatus = "restored"
	BackupRestoreFail BackupStatus = "restore_failed"
)

type DatabaseEngine string

const (
	DatabasePostgres DatabaseEngine = "postgres"
	DatabaseMySQL    DatabaseEngine = "mysql"
	DatabaseMariaDB  DatabaseEngine = "mariadb"
	DatabaseMongoDB  DatabaseEngine = "mongodb"
	DatabaseRedis    DatabaseEngine = "redis"
	DatabaseLibSQL   DatabaseEngine = "libsql"
)

type BackupType string

const (
	BackupTypeServer   BackupType = "server"
	BackupTypeDatabase BackupType = "database"
	BackupTypeVolume   BackupType = "volume"
	BackupTypeApp      BackupType = "app"
)

type Backup struct {
	ID          string          `json:"id"`
	ServerID    string          `json:"serverId"`
	Name        string          `json:"name"`
	Status      BackupStatus    `json:"status"`
	Size        int64           `json:"size,omitempty"`
	Checksum    string          `json:"checksum,omitempty"`
	Storage     string          `json:"storage"`
	Path        string          `json:"path"`
	Locked      bool            `json:"locked"`
	Compressed  bool            `json:"compressed,omitempty"`
	Encrypted   bool            `json:"encrypted,omitempty"`
	Nonce       string          `json:"nonce,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	CompletedAt *time.Time      `json:"completedAt,omitempty"`
	ExpiresAt   *time.Time      `json:"expiresAt,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type BackupManifest struct {
	Version           int               `json:"version"`
	ChecksumAlgorithm string            `json:"checksumAlgorithm"`
	ChecksumValue     string            `json:"checksumValue"`
	FileCount         int               `json:"fileCount"`
	TotalSizeBytes    int64             `json:"totalSizeBytes"`
	SourceType        string            `json:"sourceType"`
	SourceID          string            `json:"sourceId"`
	Engine            string            `json:"engine,omitempty"`
	DatabaseName      string            `json:"databaseName,omitempty"`
	VolumeName        string            `json:"volumeName,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type StorageReceipt struct {
	Adapter   string `json:"adapter"`
	Path      string `json:"path"`
	ETag      string `json:"etag"`
	VersionID string `json:"versionId,omitempty"`
}

type UploadOptions struct {
	Compress      bool
	EncryptionKey []byte
}

type DownloadOptions struct {
	Compressed    bool
	Encrypted     bool
	EncryptionKey []byte
}

type CreateBackupRequest struct {
	Name    string `json:"name"`
	Locked  bool   `json:"locked"`
	Storage string `json:"storage"`
}

type CreateDatabaseBackupRequest struct {
	ServerID     string         `json:"serverId"`
	BackupName   string         `json:"backupName"`
	DatabaseID   string         `json:"databaseId"`
	Engine       DatabaseEngine `json:"engine"`
	DatabaseName string         `json:"databaseName"`
	Locked       bool           `json:"locked"`
	Storage      string         `json:"storage"`
}

type CreateVolumeBackupRequest struct {
	ServerID        string   `json:"serverId"`
	BackupName      string   `json:"backupName"`
	VolumeName      string   `json:"volumeName"`
	VolumeMountPath string   `json:"volumeMountPath"`
	IncludePaths    []string `json:"includePaths,omitempty"`
	ExcludePaths    []string `json:"excludePaths,omitempty"`
	Locked          bool     `json:"locked"`
	Storage         string   `json:"storage"`
}

type ProviderFactory func(config map[string]string) (StorageAdapter, error)

var providerFactories map[string]ProviderFactory

func RegisterProvider(name string, factory ProviderFactory) {
	if providerFactories == nil {
		providerFactories = make(map[string]ProviderFactory)
	}
	providerFactories[name] = factory
}

func GetProvider(name string, config map[string]string) (StorageAdapter, error) {
	factory, ok := providerFactories[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}
	return factory(config)
}

func RegisteredProviders() []string {
	var names []string
	for name := range providerFactories {
		names = append(names, name)
	}
	return names
}

type Service struct {
	store                *store.Store
	adapters             map[string]StorageAdapter
	defaultAdapter       string
	defaultRetentionDays int
	retryCfg             retryConfig
	cronParser           cron.Parser
}

func New(store *store.Store) *Service {
	return &Service{
		store:                store,
		adapters:             make(map[string]StorageAdapter),
		defaultRetentionDays: 30,
		retryCfg: retryConfig{
			maxRetries:  defaultMaxRetries,
			baseBackoff: defaultBaseBackoff,
			maxBackoff:  defaultMaxBackoff,
		},
		cronParser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

func (s *Service) SetLogger(logger *log.Logger) {
	_ = logger
}

func (s *Service) SetRetentionDays(days int) {
	if days > 0 {
		s.defaultRetentionDays = days
	}
}

func (s *Service) RegisterAdapter(adapter StorageAdapter) {
	s.adapters[adapter.Name()] = adapter
	if s.defaultAdapter == "" {
		s.defaultAdapter = adapter.Name()
	}
}

func (s *Service) adapter(name string) (StorageAdapter, error) {
	if name == "" {
		name = s.defaultAdapter
	}
	a, ok := s.adapters[name]
	if !ok {
		return nil, fmt.Errorf("storage adapter %q not registered", name)
	}
	return a, nil
}

func (s *Service) buildStoragePath(serverID, name string) string {
	return "backups/" + serverID + "/" + name
}

func (s *Service) CreateBackup(ctx context.Context, serverID string, req CreateBackupRequest) (*Backup, error) {
	storage := req.Storage
	if storage == "" {
		storage = s.defaultAdapter
	}
	now := time.Now().UTC()
	backup := &Backup{
		ID:        uuid.NewString(),
		ServerID:  serverID,
		Name:      req.Name,
		Status:    BackupPending,
		Storage:   storage,
		Locked:    req.Locked,
		CreatedAt: now,
	}
	if s.store != nil {
		_, err := s.store.UpsertBackup(ctx, serverID, store.UpsertBackupRequest{
			UUID:   backup.ID,
			Name:   backup.Name,
			Status: string(backup.Status),
		}, nil)
		if err != nil {
			return nil, err
		}
	}
	return backup, nil
}

func (s *Service) GetBackup(ctx context.Context, serverID, name string) (store.Backup, error) {
	if s.store != nil {
		return s.store.GetBackupByName(ctx, serverID, name)
	}
	return store.Backup{}, nil
}

func (s *Service) ListBackups(ctx context.Context, serverID string, page, perPage int) ([]store.Backup, error) {
	if s.store != nil {
		return s.store.ListBackups(ctx, serverID, page, perPage)
	}
	return nil, nil
}

func (s *Service) UploadBackup(ctx context.Context, serverID, name, storage string, data io.Reader, checksum string, size int64) error {
	return s.UploadBackupWithOptions(ctx, serverID, name, storage, data, checksum, size, UploadOptions{})
}

func (s *Service) UploadBackupWithOptions(ctx context.Context, serverID, name, storage string, data io.Reader, checksum string, size int64, opts UploadOptions) error {
	adapter, err := s.adapter(storage)
	if err != nil {
		return err
	}

	backupPath := name
	if !strings.HasSuffix(backupPath, ".tar.gz") && !strings.HasSuffix(backupPath, ".zip") {
		backupPath = backupPath + ".tar.gz"
	}

	var compressed bool
	if opts.Compress {
		compressed = true
		compressedReader, compErr := CompressReader(data, backupPath)
		if compErr != nil {
			return fmt.Errorf("compress backup: %w", compErr)
		}
		data = compressedReader
	}

	var encrypted bool
	var nonceHex string
	if len(opts.EncryptionKey) > 0 {
		encrypted = true
		encReader, encErr := EncryptReader(data, opts.EncryptionKey)
		if encErr != nil {
			return fmt.Errorf("encrypt backup: %w", encErr)
		}
		data = encReader
	}

	dataBytes, err := io.ReadAll(data)
	if err != nil {
		if s.store != nil {
			s.store.MarkBackupStatus(ctx, serverID, name, string(BackupFailed), nil)
		}
		return fmt.Errorf("read backup data: %w", err)
	}

	if encrypted && len(dataBytes) >= nonceSize {
		nonceHex = hex.EncodeToString(dataBytes[:nonceSize])
	}

	err = withRetry(ctx, s.retryCfg, func(ctx context.Context) error {
		return adapter.Upload(ctx, backupPath, dataBytes)
	})
	if err != nil {
		if s.store != nil {
			s.store.MarkBackupStatus(ctx, serverID, name, string(BackupFailed), nil)
		}
		return fmt.Errorf("upload backup: %w", err)
	}

	receipt := StorageReceipt{
		Adapter: storage,
		Path:    backupPath,
	}
	receiptBytes, _ := json.Marshal(receipt)

	now := time.Now().UTC()
	if s.store != nil {
		_, err = s.store.UpsertBackup(ctx, serverID, store.UpsertBackupRequest{
			Name:             name,
			Checksum:         checksum,
			Size:             size,
			Status:           string(BackupCompleted),
			CompletedAt:      &now,
			StorageReceipt:   receiptBytes,
			ChecksumVerified: checksum != "",
			Compressed:       compressed,
			Encrypted:        encrypted,
			Nonce:            nonceHex,
		}, nil)
		if err != nil {
			return fmt.Errorf("update backup record: %w", err)
		}
	}

	return nil
}

func (s *Service) DownloadBackup(ctx context.Context, serverID, name, storage string) ([]byte, error) {
	return s.DownloadBackupWithOptions(ctx, serverID, name, storage, DownloadOptions{})
}

func (s *Service) DownloadBackupWithOptions(ctx context.Context, serverID, name, storage string, opts DownloadOptions) ([]byte, error) {
	adapter, err := s.adapter(storage)
	if err != nil {
		return nil, err
	}

	backupPath := name
	if !strings.Contains(backupPath, ".") {
		backupPath = backupPath + ".tar.gz"
	}

	var data []byte
	err = withRetry(ctx, s.retryCfg, func(ctx context.Context) error {
		var innerErr error
		data, innerErr = adapter.Download(ctx, backupPath)
		return innerErr
	})
	if err != nil {
		return nil, fmt.Errorf("download backup: %w", err)
	}

	if opts.Encrypted && len(opts.EncryptionKey) > 0 {
		decrypted, decErr := Decrypt(data, opts.EncryptionKey)
		if decErr != nil {
			return nil, fmt.Errorf("decrypt backup: %w", decErr)
		}
		data = decrypted
	}

	if opts.Compressed {
		decompressed, compErr := Decompress(data)
		if compErr != nil {
			return nil, fmt.Errorf("decompress backup: %w", compErr)
		}
		data = decompressed
	}

	return data, nil
}

func (s *Service) DownloadFromStorage(ctx context.Context, storageName string, path string) ([]byte, error) {
	adapter, err := s.adapter(storageName)
	if err != nil {
		return nil, err
	}
	var data []byte
	err = withRetry(ctx, s.retryCfg, func(ctx context.Context) error {
		var innerErr error
		data, innerErr = adapter.Download(ctx, path)
		return innerErr
	})
	if err != nil {
		return nil, fmt.Errorf("download from storage %q: %w", storageName, err)
	}
	if data == nil {
		return nil, fmt.Errorf("download returned nil data from storage %q", storageName)
	}
	return data, nil
}

func (s *Service) DeleteBackupFromStorage(ctx context.Context, serverID, name, storage string) error {
	adapter, err := s.adapter(storage)
	if err != nil {
		return err
	}

	backupPath := name
	if !strings.Contains(backupPath, ".") {
		backupPath = backupPath + ".tar.gz"
	}

	err = withRetry(ctx, s.retryCfg, func(ctx context.Context) error {
		return adapter.Delete(ctx, backupPath)
	})
	if err != nil {
		return fmt.Errorf("delete backup from storage: %w", err)
	}

	return nil
}

func (s *Service) VerifyChecksum(data []byte, expected string) error {
	if expected == "" {
		return nil
	}
	hasher := sha256.New()
	hasher.Write(data)
	actual := fmt.Sprintf("%x", hasher.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

func (s *Service) VerifyChecksumFromStore(ctx context.Context, backup store.Backup) error {
	if backup.Checksum == "" {
		return nil
	}

	adapterName := ""
	var receipt StorageReceipt
	if len(backup.StorageReceipt) > 0 {
		if json.Unmarshal(backup.StorageReceipt, &receipt) == nil {
			adapterName = receipt.Adapter
		}
	}

	adapter, err := s.adapter(adapterName)
	if err != nil {
		adapter, err = s.adapter("")
		if err != nil {
			return fmt.Errorf("no adapter for checksum verification: %w", err)
		}
	}

	storagePath := s.buildStoragePath(backup.ServerID, backup.Name)
	if receipt.Path != "" {
		storagePath = receipt.Path
	}

	data, err := adapter.Download(ctx, storagePath)
	if err != nil {
		return fmt.Errorf("download for checksum verification: %w", err)
	}

	hasher := sha256.New()
	hasher.Write(data)
	actual := fmt.Sprintf("%x", hasher.Sum(nil))
	if !strings.EqualFold(actual, backup.Checksum) {
		return fmt.Errorf("checksum mismatch for backup %s/%s: expected %s, got %s", backup.ServerID, backup.Name, backup.Checksum, actual)
	}
	return nil
}

func (s *Service) VerifyStorageReceipt(ctx context.Context, backup store.Backup) (*StorageReceipt, error) {
	adapterName := ""
	var storedReceipt StorageReceipt
	if len(backup.StorageReceipt) > 0 {
		if json.Unmarshal(backup.StorageReceipt, &storedReceipt) == nil {
			adapterName = storedReceipt.Adapter
		}
	}

	adapter, err := s.adapter(adapterName)
	if err != nil {
		adapter, err = s.adapter("")
		if err != nil {
			return nil, fmt.Errorf("no adapter for receipt verification: %w", err)
		}
		adapterName = s.defaultAdapter
	}

	storagePath := s.buildStoragePath(backup.ServerID, backup.Name)
	if storedReceipt.Path != "" {
		storagePath = storedReceipt.Path
	}

	exists, err := adapter.Exists(ctx, storagePath)
	if err != nil {
		return nil, fmt.Errorf("check backup existence: %w", err)
	}

	receipt := &StorageReceipt{
		Adapter: adapterName,
		Path:    storagePath,
		ETag:    "",
	}

	if !exists {
		return receipt, fmt.Errorf("backup %s/%s not found in storage", backup.ServerID, backup.Name)
	}

	data, err := adapter.Download(ctx, storagePath)
	if err != nil {
		return receipt, fmt.Errorf("download for receipt verification: %w", err)
	}

	if backup.Checksum != "" {
		hasher := sha256.New()
		hasher.Write(data)
		computed := fmt.Sprintf("%x", hasher.Sum(nil))
		if strings.EqualFold(computed, backup.Checksum) {
			receipt.ETag = computed
		} else {
			return receipt, fmt.Errorf("storage receipt checksum mismatch")
		}
	}

	return receipt, nil
}

func (s *Service) GenerateManifest(ctx context.Context, serverID, backupName, sourceType, sourceID string, fileCount int, totalSize int64, engine, dbName, volumeName string, meta map[string]string) (*BackupManifest, error) {
	manifest := &BackupManifest{
		Version:           1,
		ChecksumAlgorithm: "sha256",
		FileCount:         fileCount,
		TotalSizeBytes:    totalSize,
		SourceType:        sourceType,
		SourceID:          sourceID,
		Engine:            engine,
		DatabaseName:      dbName,
		VolumeName:        volumeName,
		Metadata:          meta,
	}
	return manifest, nil
}

func (s *Service) RestoreFromStorage(ctx context.Context, serverID, backupName string, targetWriter io.Writer) (int64, error) {
	if s.store == nil {
		return 0, fmt.Errorf("store unavailable")
	}
	b, err := s.store.GetBackupByName(ctx, serverID, backupName)
	if err != nil {
		return 0, fmt.Errorf("backup not found: %w", err)
	}
	if b.Status != "completed" {
		return 0, fmt.Errorf("backup status is %s, cannot restore", b.Status)
	}

	var actorID *string
	if err := s.store.MarkBackupStatus(ctx, serverID, backupName, "restoring", actorID); err != nil {
		return 0, err
	}

	adapterName := s.defaultAdapter
	storagePath := s.buildStoragePath(serverID, backupName)
	var receipt StorageReceipt
	if len(b.StorageReceipt) > 0 {
		if json.Unmarshal(b.StorageReceipt, &receipt) == nil {
			if receipt.Adapter != "" {
				adapterName = receipt.Adapter
			}
			if receipt.Path != "" {
				storagePath = receipt.Path
			}
		}
	}

	data, err := s.DownloadFromStorage(ctx, adapterName, storagePath)
	if err != nil {
		_ = s.store.MarkBackupStatus(ctx, serverID, backupName, "restore_failed", actorID)
		return 0, err
	}

	if b.Checksum != "" {
		if err := s.VerifyChecksum(data, b.Checksum); err != nil {
			_ = s.store.MarkBackupStatus(ctx, serverID, backupName, "restore_failed", actorID)
			return 0, fmt.Errorf("verification failed: %w", err)
		}
	}

	written, err := targetWriter.Write(data)
	if err != nil {
		_ = s.store.MarkBackupStatus(ctx, serverID, backupName, "restore_failed", actorID)
		return 0, fmt.Errorf("write restored data: %w", err)
	}

	now := time.Now().UTC()
	if _, err := s.store.UpsertBackup(ctx, serverID, store.UpsertBackupRequest{
		Name:          backupName,
		Status:        "restored",
		RestoreCount:  b.RestoreCount + 1,
		LastRestoreAt: &now,
	}, actorID); err != nil {
		return int64(written), err
	}

	return int64(written), nil
}

func (s *Service) CreateDatabaseBackup(ctx context.Context, req CreateDatabaseBackupRequest) (*Backup, error) {
	storage := req.Storage
	if storage == "" {
		storage = s.defaultAdapter
	}
	now := time.Now().UTC()
	backup := &Backup{
		ID:        uuid.NewString(),
		ServerID:  req.ServerID,
		Name:      req.BackupName,
		Status:    BackupPending,
		Storage:   storage,
		CreatedAt: now,
	}

	if s.store != nil {
		_, err := s.store.UpsertBackup(ctx, req.ServerID, store.UpsertBackupRequest{
			UUID:         backup.ID,
			Name:         backup.Name,
			Status:       string(backup.Status),
			SourceType:   string(BackupTypeDatabase),
			SourceID:     req.DatabaseID,
			DatabaseType: string(req.Engine),
		}, nil)
		if err != nil {
			return nil, err
		}
	}
	return backup, nil
}

func (s *Service) CreateVolumeBackup(ctx context.Context, req CreateVolumeBackupRequest) (*Backup, error) {
	storage := req.Storage
	if storage == "" {
		storage = s.defaultAdapter
	}
	now := time.Now().UTC()
	backup := &Backup{
		ID:        uuid.NewString(),
		ServerID:  req.ServerID,
		Name:      req.BackupName,
		Status:    BackupPending,
		Storage:   storage,
		CreatedAt: now,
	}

	if s.store != nil {
		_, err := s.store.UpsertBackup(ctx, req.ServerID, store.UpsertBackupRequest{
			UUID:       backup.ID,
			Name:       backup.Name,
			Status:     string(backup.Status),
			SourceType: string(BackupTypeVolume),
			SourceID:   req.VolumeName,
			VolumeName: req.VolumeName,
		}, nil)
		if err != nil {
			return nil, err
		}
	}
	return backup, nil
}

func (s *Service) CleanupExpiredBackups(ctx context.Context) (int64, error) {
	if s.store == nil {
		return 0, nil
	}
	var cleaned int64

	policies, err := s.ListAllEnabledPolicies(ctx)
	if err == nil {
		for _, p := range policies {
			backups, listErr := s.store.ListBackups(ctx, p.ServerID, 1, 1000)
			if listErr != nil {
				continue
			}
			for _, b := range backups {
				if b.Status != "completed" || b.IsLocked {
					continue
				}
				if b.CreatedAt.Before(time.Now().AddDate(0, 0, -p.RetentionDays)) {
					if delErr := s.DeleteBackupFromStorage(ctx, b.ServerID, b.Name, p.Storage); delErr != nil {
						continue
					}
					s.store.DeleteBackup(ctx, b.ServerID, b.Name, nil)
					cleaned++
				}
			}
		}
	}

	expired, listErr := s.store.ListExpiredBackups(ctx)
	if listErr == nil {
		for _, b := range expired {
			if b.Status != "completed" || b.IsLocked {
				continue
			}
			storage := s.defaultAdapter
			var receipt StorageReceipt
			if len(b.StorageReceipt) > 0 {
				if json.Unmarshal(b.StorageReceipt, &receipt) == nil && receipt.Adapter != "" {
					storage = receipt.Adapter
				}
			}
			if delErr := s.DeleteBackupFromStorage(ctx, b.ServerID, b.Name, storage); delErr != nil {
				continue
			}
			if dbErr := s.store.DeleteBackup(ctx, b.ServerID, b.Name, nil); dbErr != nil {
				continue
			}
			cleaned++
		}
	}

	return cleaned, nil
}

func (s *Service) EnforceRetentionPolicy(ctx context.Context, serverID string, policy store.BackupPolicy) error {
	if s.store == nil {
		return nil
	}
	backups, err := s.store.ListBackups(ctx, serverID, 1, 1000)
	if err != nil {
		return err
	}

	var completed []store.Backup
	for _, b := range backups {
		if b.Status == "completed" {
			completed = append(completed, b)
		}
	}
	if len(completed) == 0 {
		return nil
	}

	now := time.Now()
	keep := make(map[string]bool, len(completed))
	for _, b := range completed {
		keep[b.Name] = true
	}

	// Rule 1: MaxBackups - keep only the newest N
	if policy.MaxBackups > 0 && len(completed) > policy.MaxBackups {
		keepNewest := make(map[string]bool)
		for i := len(completed) - 1; i >= 0 && i >= len(completed)-policy.MaxBackups; i-- {
			keepNewest[completed[i].Name] = true
		}
		for name := range keep {
			if !keepNewest[name] {
				delete(keep, name)
			}
		}
	}

	// Rule 2: RetentionDays - keep backups within the retention window
	if policy.RetentionDays > 0 {
		for _, b := range completed {
			if now.Sub(b.CreatedAt).Hours() > float64(policy.RetentionDays)*24 {
				delete(keep, b.Name)
			}
		}
	}

	// Rule 3: KeepDaily / KeepWeekly / KeepMonthly (from the beacon retention model)
	for _, period := range []struct {
		loHours, hiHours int
		max              int
	}{
		{0, 24, policy.MaxBackups},     // KeepDaily maps via MaxBackups
		{24, 168, policy.MaxBackups / 7},  // approximate KeepWeekly
		{168, 720, policy.MaxBackups / 30}, // approximate KeepMonthly
	} {
		if period.max <= 0 {
			continue
		}
		count := 0
		for _, b := range completed {
			ageHours := int(now.Sub(b.CreatedAt).Hours())
			if ageHours >= period.loHours && ageHours < period.hiHours {
				keep[b.Name] = true
				count++
				if count >= period.max {
					break
				}
			}
		}
	}

	// Delete anything not marked for keeping
	for _, b := range completed {
		if !keep[b.Name] {
			if delErr := s.DeleteBackupFromStorage(ctx, serverID, b.Name, policy.Storage); delErr != nil {
				continue
			}
			s.store.DeleteBackup(ctx, serverID, b.Name, nil)
		}
	}

	return nil
}

func (s *Service) CreatePolicy(ctx context.Context, p *store.BackupPolicy) error {
	if s.store != nil {
		return s.store.CreateBackupPolicy(ctx, p)
	}
	return nil
}

func (s *Service) GetPolicy(ctx context.Context, id string) (store.BackupPolicy, error) {
	if s.store != nil {
		return s.store.GetBackupPolicy(ctx, id)
	}
	return store.BackupPolicy{}, nil
}

func (s *Service) ListPolicies(ctx context.Context, serverID string) ([]store.BackupPolicy, error) {
	if s.store != nil {
		return s.store.ListBackupPolicies(ctx, serverID)
	}
	return nil, nil
}

func (s *Service) UpdatePolicy(ctx context.Context, p *store.BackupPolicy) error {
	if s.store != nil {
		return s.store.UpdateBackupPolicy(ctx, p)
	}
	return nil
}

func (s *Service) DeletePolicy(ctx context.Context, id string) error {
	if s.store != nil {
		return s.store.DeleteBackupPolicy(ctx, id)
	}
	return nil
}

func (s *Service) LockPolicy(ctx context.Context, id string) error {
	if s.store != nil {
		return s.store.LockBackupPolicy(ctx, id, nil)
	}
	return nil
}

func (s *Service) UnlockPolicy(ctx context.Context, id string) error {
	if s.store != nil {
		return s.store.UnlockBackupPolicy(ctx, id, nil)
	}
	return nil
}

func (s *Service) ListAppPolicies(ctx context.Context, appID string) ([]store.BackupPolicy, error) {
	if s.store != nil {
		return s.store.ListBackupPoliciesByApp(ctx, appID)
	}
	return nil, nil
}

func (s *Service) ListDatabasePolicies(ctx context.Context, databaseID string) ([]store.BackupPolicy, error) {
	if s.store != nil {
		return s.store.ListBackupPoliciesByDatabase(ctx, databaseID)
	}
	return nil, nil
}

func (s *Service) CleanupOrphanedPolicies(ctx context.Context) (int64, error) {
	if s.store != nil {
		return s.store.DeleteOrphanedBackupPolicies(ctx)
	}
	return 0, nil
}

func (s *Service) CleanupPoliciesByApp(ctx context.Context, appID string) (int64, error) {
	if s.store != nil {
		return s.store.CleanupBackupPoliciesByApp(ctx, appID)
	}
	return 0, nil
}

func (s *Service) CleanupPoliciesByDatabase(ctx context.Context, databaseID string) (int64, error) {
	if s.store != nil {
		return s.store.CleanupBackupPoliciesByDatabase(ctx, databaseID)
	}
	return 0, nil
}

func (s *Service) ListAllEnabledPolicies(ctx context.Context) ([]store.BackupPolicy, error) {
	if s.store != nil {
		return s.store.ListAllEnabledBackupPolicies(ctx)
	}
	return nil, nil
}

func (s *Service) ListAllPolicies(ctx context.Context) ([]store.BackupPolicy, error) {
	if s.store != nil {
		return s.store.ListAllBackupPolicies(ctx)
	}
	return nil, nil
}

func (s *Service) NextCronRun(policy store.BackupPolicy, from time.Time) (time.Time, error) {
	sch, err := s.cronParser.Parse(policy.Interval)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse cron %q: %w", policy.Interval, err)
	}
	return sch.Next(from), nil
}

func (s *Service) log(msg string, args ...any) {
	if len(args) == 0 {
		log.Printf("[backup] %s", msg)
		return
	}
	pairs := make([]string, 0, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		pairs = append(pairs, fmt.Sprintf("%v=%v", args[i], args[i+1]))
	}
	log.Printf("[backup] %s (%s)", msg, strings.Join(pairs, ", "))
}
