package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

type mockBackupEntry struct {
	info BackupInfo
	data []byte
}

type MockBackup struct {
	mu      sync.Mutex
	servers map[string][]mockBackupEntry
}

func NewMockBackup() *MockBackup {
	return &MockBackup{
		servers: make(map[string][]mockBackupEntry),
	}
}

func (m *MockBackup) Type() AdapterType { return "mock" }

func (m *MockBackup) AddBackup(serverID, name string, data []byte, created time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hasher := sha256.New()
	hasher.Write(data)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	entry := mockBackupEntry{
		info: BackupInfo{
			UUID:        name,
			Name:        name,
			Checksum:    checksum,
			Size:        int64(len(data)),
			Status:      "completed",
			Created:     created,
			CompletedAt: created,
			Adapter:     "mock",
		},
		data: data,
	}
	m.servers[serverID] = append(m.servers[serverID], entry)
}

func (m *MockBackup) AddBackupWithChecksum(serverID, name string, data []byte, checksum string, created time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := mockBackupEntry{
		info: BackupInfo{
			UUID:        name,
			Name:        name,
			Checksum:    checksum,
			Size:        int64(len(data)),
			Status:      "completed",
			Created:     created,
			CompletedAt: created,
			Adapter:     "mock",
		},
		data: data,
	}
	m.servers[serverID] = append(m.servers[serverID], entry)
}

func (m *MockBackup) Create(ctx context.Context, serverRoot, backupDir, name string, ignored []string) (*BackupInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data := []byte(fmt.Sprintf("backup-data-%s-%s", backupDir, name))
	hasher := sha256.New()
	hasher.Write(data)
	checksum := hex.EncodeToString(hasher.Sum(nil))
	now := time.Now().UTC()

	entry := mockBackupEntry{
		info: BackupInfo{
			UUID:        name,
			Name:        name,
			Checksum:    checksum,
			Size:        int64(len(data)),
			Status:      "completed",
			Created:     now,
			CompletedAt: now,
			Adapter:     "mock",
		},
		data: data,
	}
	m.servers[backupDir] = append(m.servers[backupDir], entry)
	return &entry.info, nil
}

func (m *MockBackup) List(backupDir string) ([]BackupInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries := m.servers[backupDir]
	result := make([]BackupInfo, len(entries))
	for i, e := range entries {
		result[i] = e.info
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Created.Before(result[j].Created)
	})
	return result, nil
}

func (m *MockBackup) Get(backupDir, name string) (*BackupInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range m.servers[backupDir] {
		if e.info.Name == name {
			info := e.info
			return &info, nil
		}
	}
	return nil, fmt.Errorf("backup %s not found", name)
}

func (m *MockBackup) Delete(backupDir, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries := m.servers[backupDir]
	for i, e := range entries {
		if e.info.Name == name {
			m.servers[backupDir] = append(entries[:i], entries[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("backup %s not found", name)
}

func (m *MockBackup) Restore(ctx context.Context, backupDir, name, serverRoot string, truncate bool) error {
	return nil
}

func (m *MockBackup) Download(backupDir, name string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range m.servers[backupDir] {
		if e.info.Name == name {
			return io.NopCloser(bytes.NewReader(e.data)), nil
		}
	}
	return nil, fmt.Errorf("backup %s not found", name)
}
