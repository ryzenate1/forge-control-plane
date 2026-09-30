package recovery

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"
)

// DaemonBackupRestoreExecutor restores an archive only after the destination
// daemon independently confirms it can access the exact recorded archive. This
// supports shared backup storage (for example, identically configured S3), not
// source-node-local archives.
type DaemonBackupRestoreExecutor struct {
	store       *store.Store
	daemon      *daemon.Client
	provisioner interface {
		ProvisionRecoveredServer(context.Context, string) error
	}
}

func NewDaemonBackupRestoreExecutor(s *store.Store, client *daemon.Client, provisioner interface {
	ProvisionRecoveredServer(context.Context, string) error
}) *DaemonBackupRestoreExecutor {
	return &DaemonBackupRestoreExecutor{store: s, daemon: client, provisioner: provisioner}
}

func (e *DaemonBackupRestoreExecutor) VerifyAndRestore(ctx context.Context, item store.RecoveryItem) error {
	if e == nil || e.store == nil || e.daemon == nil {
		return errors.New("backup recovery executor unavailable")
	}
	if item.TargetNodeID == "" || item.ServerID == "" || item.SourceBackupName == "" || item.SourceBackupChecksum == "" || item.SourceBackupSize <= 0 {
		return errors.New("recovery item has no verified backup restore source")
	}
	target, err := e.store.RecoveryRestoreTarget(ctx, item.TargetNodeID, item.ServerID)
	if err != nil {
		return fmt.Errorf("load recovery target: %w", err)
	}
	backups, err := e.daemon.ListBackups(ctx, target.NodeURL, target.NodeToken, item.ServerID)
	if err != nil {
		return fmt.Errorf("verify target backup access: %w", err)
	}
	for _, backup := range backups {
		if backup.Name == item.SourceBackupName && backup.Status == "completed" &&
			backup.Size == item.SourceBackupSize && strings.EqualFold(backup.Checksum, item.SourceBackupChecksum) {

			// Verify checksum before restore
			checksumErr := e.verifyBackupChecksum(ctx, target, item.ServerID, item.SourceBackupName, item.SourceBackupChecksum)
			if checksumErr != nil {
				return fmt.Errorf("pre-restore checksum verification failed: %w", checksumErr)
			}

			if err := e.daemon.RestoreBackup(ctx, target.NodeURL, target.NodeToken, item.ServerID, item.SourceBackupName, true); err != nil {
				return fmt.Errorf("restore verified backup: %w", err)
			}
			if e.provisioner == nil {
				return errors.New("recovery workload provisioner unavailable")
			}
			if err := e.store.BeginRecoveryOwnership(ctx, item.ID); err != nil {
				return fmt.Errorf("begin recovery ownership: %w", err)
			}
			if err := e.provisioner.ProvisionRecoveredServer(ctx, item.ServerID); err != nil {
				rollbackErr := e.store.RollbackRecoveryOwnership(ctx, item.ID)
				return errors.Join(fmt.Errorf("provision recovered workload: %w", err), rollbackErr)
			}
			if err := e.store.CompleteRecoveryOwnership(ctx, item.ID); err != nil {
				return fmt.Errorf("complete recovery ownership: %w", err)
			}

			// Mark backup as restored
			_, _ = e.store.UpsertBackup(ctx, item.ServerID, store.UpsertBackupRequest{
				Name:   item.SourceBackupName,
				Status: "restored",
			}, nil)

			return nil
		}
	}
	return errors.New("target daemon cannot verify access to the planned backup archive")
}

func (e *DaemonBackupRestoreExecutor) verifyBackupChecksum(ctx context.Context, target store.ServerControlTarget, serverID, backupName, expectedChecksum string) error {
	// Download the backup archive to verify checksum
	reader, err := e.daemon.DownloadBackup(ctx, target.NodeURL, target.NodeToken, serverID, backupName)
	if err != nil {
		return fmt.Errorf("download backup for checksum verification: %w", err)
	}
	defer reader.Close()

	hasher := sha256.New()
	buf := make([]byte, 32*1024)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			hasher.Write(buf[:n])
		}
		if readErr != nil {
			if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
				return readErr
			}
			break
		}
	}

	computed := fmt.Sprintf("%x", hasher.Sum(nil))
	if !strings.EqualFold(computed, expectedChecksum) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, computed)
	}
	return nil
}

var _ BackupRestoreExecutor = (*DaemonBackupRestoreExecutor)(nil)
