package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
)

type VerificationResult struct {
	BackupID    string
	StartedAt   time.Time
	CompletedAt time.Time
	Status      BackupStatus
	Error       string
	Checksum    string
	SizeBytes   int64
	Duration    time.Duration
}

type IntegrityReport struct {
	TotalBackups   int                  `json:"totalBackups"`
	ValidBackups   int                  `json:"validBackups"`
	InvalidBackups int                  `json:"invalidBackups"`
	Results        []VerificationResult `json:"results"`
}

func VerifyBackup(ctx context.Context, adapter BackupInterface, serverID, backupID string) (*VerificationResult, error) {
	info, err := adapter.Get(serverID, backupID)
	if err != nil {
		return nil, fmt.Errorf("get backup info: %w", err)
	}

	now := time.Now().UTC()

	reader, err := adapter.Download(serverID, backupID)
	if err != nil {
		return &VerificationResult{
			BackupID:    backupID,
			StartedAt:   now,
			CompletedAt: now,
			Status:      BackupStatusFailed,
			Error:       err.Error(),
			Checksum:    info.Checksum,
		}, nil
	}
	defer reader.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return &VerificationResult{
			BackupID:    backupID,
			StartedAt:   now,
			CompletedAt: now,
			Status:      BackupStatusFailed,
			Error:       err.Error(),
			Checksum:    info.Checksum,
		}, nil
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	status := BackupStatusCompleted
	if !strings.EqualFold(actualChecksum, info.Checksum) {
		status = BackupStatusFailed
	}

	return &VerificationResult{
		BackupID:    backupID,
		StartedAt:   now,
		CompletedAt: now,
		Status:      status,
		Checksum:    info.Checksum,
		SizeBytes:   info.Size,
	}, nil
}

func VerifyAllBackups(ctx context.Context, adapter BackupInterface, serverID string) ([]VerificationResult, error) {
	backups, err := adapter.List(serverID)
	if err != nil {
		return nil, fmt.Errorf("list backups for verification: %w", err)
	}

	results := make([]VerificationResult, 0, len(backups))
	for _, b := range backups {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := VerifyBackup(ctx, adapter, serverID, b.Name)
		if err != nil {
			return nil, fmt.Errorf("verify backup %s: %w", b.Name, err)
		}
		results = append(results, *result)
	}
	return results, nil
}

func GenerateIntegrityReport(ctx context.Context, adapter BackupInterface, serverID string) (*IntegrityReport, error) {
	results, err := VerifyAllBackups(ctx, adapter, serverID)
	if err != nil {
		return nil, err
	}

	report := &IntegrityReport{
		TotalBackups: len(results),
		Results:      results,
	}
	for _, r := range results {
		if r.Status == BackupStatusCompleted {
			report.ValidBackups++
		} else {
			report.InvalidBackups++
		}
	}
	return report, nil
}
