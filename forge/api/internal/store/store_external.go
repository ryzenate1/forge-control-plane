package store

import (
	"context"
	"errors"
)

func (s *Store) GetUserByExternalID(ctx context.Context, externalID string) (User, error) {
	if s.db == nil {
		return User{}, errors.New("no database connection")
	}
	var user User
	err := s.db.QueryRow(ctx, `
		SELECT u.id::text, u.email,
		       CASE WHEN COALESCE(r.is_admin, FALSE) THEN 'admin' ELSE COALESCE(r.key, 'user') END AS role,
		       COALESCE(u.external_id, ''),
		       COALESCE(u.cpu_limit, 0), COALESCE(u.memory_mb_limit, 0),
		       COALESCE(u.disk_mb_limit, 0), COALESCE(u.backup_limit, 0),
		       COALESCE(u.database_limit, 0), COALESCE(u.allocation_limit, 0),
		       COALESCE(u.subuser_limit, 0), COALESCE(u.schedule_limit, 0),
		       COALESCE(u.server_limit, 0), u.use_totp, u.session_version, u.disabled
		FROM users u
		LEFT JOIN LATERAL (
			SELECT r.key, r.is_admin
			FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = u.id
			ORDER BY r.is_admin DESC, ur.assigned_at ASC
			LIMIT 1
		) r ON TRUE
		WHERE u.external_id = $1 AND NOT u.disabled
	`, externalID).Scan(&user.ID, &user.Email, &user.Role,
		&user.ExternalID,
		&user.CPULimit, &user.MemoryMBLimit, &user.DiskMBLimit,
		&user.BackupLimit, &user.DatabaseLimit, &user.AllocationLimit,
		&user.SubuserLimit, &user.ScheduleLimit, &user.ServerLimit,
		&user.UseTOTP, &user.SessionVersion, &user.Disabled)
	return user, err
}

func (s *Store) GetServerByExternalID(ctx context.Context, externalID string) (Server, error) {
	if s.db == nil {
		return Server{}, errors.New("no database connection")
	}
	var server Server
	err := s.db.QueryRow(ctx, `
		SELECT s.id::text, s.name, COALESCE(s.description, ''), s.status,
		       s.desired_state::text, s.actual_state::text, s.suspended, s.transferring,
		       s.transfer_target_node_id::text, s.transfer_state, s.transfer_error, s.transfer_run_token::text,
		       s.memory_mb, s.cpu_shares, s.cpu_limit, s.disk_mb, s.database_limit, s.backup_limit,
		       s.allocation_limit, s.io_weight, s.swap_mb, COALESCE(s.threads, ''), s.oom_disabled,
		       COALESCE(NULLIF(s.docker_image, ''), (SELECT value FROM jsonb_each_text(e.docker_images) ORDER BY key LIMIT 1), ''),
		       COALESCE(NULLIF(s.startup_command, ''), e.startup),
		       s.primary_allocation_id::text, s.config_sync_pending, s.config_sync_error,
		       n.name, n.id::text, COALESCE(NULLIF(n.fqdn, ''), n.base_url), COALESCE(n.daemon_sftp, 2022),
		       u.email, u.id::text, e.name, COALESCE(s.external_id, ''),
		       COALESCE(s.uuid, ''), COALESCE(s.uuid_short, ''), s.installed_at, s.skip_scripts, s.docker_labels
		FROM servers s
		JOIN nodes n ON n.id = s.node_id
		JOIN users u ON u.id = s.owner_id
		JOIN eggs e ON e.id = s.egg_id
		WHERE s.external_id = $1
	`, externalID).Scan(
		&server.ID, &server.Name, &server.Description, &server.Status, &server.DesiredState, &server.ActualState,
		&server.Suspended, &server.Transferring, &server.TransferTargetNodeID, &server.TransferState,
		&server.TransferError, &server.TransferRunToken, &server.MemoryMB, &server.CPUShares, &server.CPULimit,
		&server.DiskMB, &server.DatabaseLimit, &server.BackupLimit, &server.AllocationLimit, &server.IOWeight,
		&server.SwapMB, &server.Threads, &server.OOMDisabled, &server.DockerImage, &server.StartupCommand,
		&server.PrimaryAllocationID, &server.ConfigSyncPending, &server.ConfigSyncError,
		&server.Node, &server.NodeID, &server.SFTPHost, &server.SFTPPort,
		&server.Owner, &server.OwnerID, &server.Template, &server.ExternalID,
		&server.Uuid, &server.UuidShort, &server.InstalledAt, &server.SkipScripts, &server.DockerLabels,
	)
	if err != nil {
		return Server{}, err
	}
	return server, nil
}
