package backup

import (
	"context"
	"crypto/sha256"
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
	CreatedAt   time.Time       `json:"createdAt"`
	CompletedAt *time.Time      `json:"completedAt,omitempty"`
	ExpiresAt   *time.Time      `json:"expiresAt,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type CreateBackupRequest struct {
	Name    string `json:"name"`
	Locked  bool   `json:"locked"`
	Storage string `json:"storage"`
}

type StorageAdapter interface {
	Name() string
	Upload(ctx context.Context, path string, data []byte) error
	Download(ctx context.Context, path string) ([]byte, error)
	Delete(ctx context.Context, path string) error
	List(ctx context.Context, prefix string) ([]string, error)
	Exists(ctx context.Context, path string) (bool, error)
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
	_, err := s.store.UpsertBackup(ctx, serverID, store.UpsertBackupRequest{
		UUID:   backup.ID,
		Name:   backup.Name,
		Status: string(backup.Status),
	}, nil)
	if err != nil {
		return nil, err
	}
	return backup, nil
}

func (s *Service) GetBackup(ctx context.Context, serverID, name string) (store.Backup, error) {
	return s.store.GetBackupByName(ctx, serverID, name)
}

func (s *Service) ListBackups(ctx context.Context, serverID string, page, perPage int) ([]store.Backup, error) {
	return s.store.ListBackups(ctx, serverID, page, perPage)
}

func (s *Service) UploadBackup(ctx context.Context, serverID, name, storage string, data io.Reader, checksum string, size int64) error {
	adapter, err := s.adapter(storage)
	if err != nil {
		return err
	}

	backupPath := name
	if !strings.HasSuffix(backupPath, ".tar.gz") && !strings.HasSuffix(backupPath, ".zip") {
		backupPath = backupPath + ".tar.gz"
	}

	dataBytes, err := io.ReadAll(data)
	if err != nil {
		s.store.MarkBackupStatus(ctx, serverID, name, string(BackupFailed), nil)
		return fmt.Errorf("read backup data: %w", err)
	}

	err = withRetry(ctx, s.retryCfg, func(ctx context.Context) error {
		return adapter.Upload(ctx, backupPath, dataBytes)
	})
	if err != nil {
		s.store.MarkBackupStatus(ctx, serverID, name, string(BackupFailed), nil)
		return fmt.Errorf("upload backup: %w", err)
	}

	now := time.Now().UTC()
	_, err = s.store.UpsertBackup(ctx, serverID, store.UpsertBackupRequest{
		Name:        name,
		Checksum:    checksum,
		Size:        size,
		Status:      string(BackupCompleted),
		CompletedAt: &now,
	}, nil)
	if err != nil {
		return fmt.Errorf("update backup record: %w", err)
	}

	return nil
}

func (s *Service) DownloadBackup(ctx context.Context, serverID, name, storage string) ([]byte, error) {
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

func (s *Service) RestoreFromStorage(ctx context.Context, serverID, backupName string, targetWriter io.Writer) (int64, error) {
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

	storagePath := "backups/" + serverID + "/" + backupName
	data, err := s.DownloadFromStorage(ctx, s.defaultAdapter, storagePath)
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

	if err := s.store.MarkBackupStatus(ctx, serverID, backupName, "restored", actorID); err != nil {
		return int64(written), err
	}

	return int64(written), nil
}

func (s *Service) CleanupExpiredBackups(ctx context.Context) (int64, error) {
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

	count, err := s.store.CleanupOldBackups(ctx, s.defaultRetentionDays, true)
	if err != nil {
		return cleaned, err
	}
	return cleaned + int64(count), nil
}

func (s *Service) EnforceRetentionPolicy(ctx context.Context, serverID string, policy store.BackupPolicy) error {
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

	if len(completed) > policy.MaxBackups {
		toRemove := len(completed) - policy.MaxBackups
		for i := 0; i < toRemove && i < len(completed); i++ {
			b := completed[i]
			if delErr := s.DeleteBackupFromStorage(ctx, serverID, b.Name, policy.Storage); delErr != nil {
				continue
			}
			s.store.DeleteBackup(ctx, serverID, b.Name, nil)
		}
	}

	return nil
}

func (s *Service) CreatePolicy(ctx context.Context, p *store.BackupPolicy) error {
	return s.store.CreateBackupPolicy(ctx, p)
}

func (s *Service) GetPolicy(ctx context.Context, id string) (store.BackupPolicy, error) {
	return s.store.GetBackupPolicy(ctx, id)
}

func (s *Service) ListPolicies(ctx context.Context, serverID string) ([]store.BackupPolicy, error) {
	return s.store.ListBackupPolicies(ctx, serverID)
}

func (s *Service) UpdatePolicy(ctx context.Context, p *store.BackupPolicy) error {
	return s.store.UpdateBackupPolicy(ctx, p)
}

func (s *Service) DeletePolicy(ctx context.Context, id string) error {
	return s.store.DeleteBackupPolicy(ctx, id)
}

func (s *Service) ListAllEnabledPolicies(ctx context.Context) ([]store.BackupPolicy, error) {
	return s.store.ListAllEnabledBackupPolicies(ctx)
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
