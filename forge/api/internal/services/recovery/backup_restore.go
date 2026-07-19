package recovery

import (
	"context"
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
			return nil
		}
	}
	return errors.New("target daemon cannot verify access to the planned backup archive")
}

var _ BackupRestoreExecutor = (*DaemonBackupRestoreExecutor)(nil)
