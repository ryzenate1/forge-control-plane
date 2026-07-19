package backup

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"gamepanel/forge/internal/store"
)

type BackupStatus string

const (
	BackupPending   BackupStatus = "pending"
	BackupRunning   BackupStatus = "running"
	BackupCompleted BackupStatus = "completed"
	BackupFailed    BackupStatus = "failed"
	BackupDeleted   BackupStatus = "deleted"
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
}

func New(store *store.Store) *Service {
	return &Service{
		store:                store,
		adapters:             make(map[string]StorageAdapter),
		defaultRetentionDays: 30,
	}
}

func (s *Service) RegisterAdapter(adapter StorageAdapter) {
	s.adapters[adapter.Name()] = adapter
	if s.defaultAdapter == "" {
		s.defaultAdapter = adapter.Name()
	}
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

func (s *Service) CleanupExpiredBackups(ctx context.Context) (int64, error) {
	count, err := s.store.CleanupOldBackups(ctx, s.defaultRetentionDays, true)
	if err != nil {
		return 0, err
	}
	return int64(count), nil
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
			if adapter, ok := s.adapters[s.defaultAdapter]; ok {
				adapter.Delete(ctx, b.Name)
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
