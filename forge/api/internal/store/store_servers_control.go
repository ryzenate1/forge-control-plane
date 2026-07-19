package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func (s *Store) SetServerPowerState(ctx context.Context, serverID, signal string) error {
	status := "stopped"
	if signal == "start" || signal == "restart" {
		status = "running"
	}
	if signal == "kill" || signal == "stop" {
		status = "stopped"
	}

	commandTag, err := s.db.Exec(ctx, `UPDATE servers SET status = $1 WHERE id = $2`, status, serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	return s.AppendAudit(ctx, nil, "server power "+signal, "server", &serverID, fmt.Sprintf(`{"status":"%s"}`, status))
}

func (s *Store) ServerControlTarget(ctx context.Context, serverID string) (ServerControlTarget, error) {
	var target ServerControlTarget
	var nodeID, tokenID, daemonToken, daemonTokenEncrypted string
	err := s.db.QueryRow(ctx, `
		SELECT s.id::text, n.base_url, n.id::text,
		       COALESCE(n.daemon_token_id, ''),
		       COALESCE(n.daemon_token, ''),
		       COALESCE(n.daemon_token_encrypted, '')
		FROM servers s
		JOIN nodes n ON n.id = s.node_id
		WHERE s.id = $1
	`, serverID).Scan(&target.ServerID, &target.NodeURL, &nodeID, &tokenID, &daemonToken, &daemonTokenEncrypted)
	if err != nil {
		return ServerControlTarget{}, err
	}
	token, err := s.decryptSecret(daemonTokenEncrypted, daemonToken, secretAAD("nodes", nodeID, "daemon_token"))
	if err != nil {
		return ServerControlTarget{}, err
	}
	if tokenID != "" && token != "" {
		target.NodeToken = tokenID + "." + token
	}
	return target, nil
}

func (s *Store) ServerProvisionTarget(ctx context.Context, serverID string) (ServerProvisionTarget, error) {
	var target ServerProvisionTarget
	var allocationIP sql.NullString
	var allocationPort sql.NullInt64
	var nodeID, tokenID, daemonToken, daemonTokenEncrypted string
	err := s.db.QueryRow(ctx, `
		SELECT s.id::text, s.egg_id::text, s.name, n.base_url,
		       n.id::text,
		       COALESCE(n.daemon_token_id, ''),
		       COALESCE(n.daemon_token, ''),
		       COALESCE(n.daemon_token_encrypted, ''),
		       COALESCE(NULLIF(s.docker_image, ''), (SELECT value FROM jsonb_each_text(e.docker_images) ORDER BY key LIMIT 1), ''),
		       COALESCE(NULLIF(s.startup_command, ''), e.startup),
		       e.install_script, e.install_container, e.install_entrypoint, e.config::text, e.file_denylist::text,
		       s.memory_mb, s.swap_mb, s.cpu_shares, s.cpu_limit, s.disk_mb, s.io_weight,
		       COALESCE(s.threads, ''), s.oom_disabled, host(a.ip), a.port,
		       s.suspended, s.installed, s.status
		FROM servers s
		JOIN nodes n ON n.id = s.node_id
		JOIN eggs e ON e.id = s.egg_id
		LEFT JOIN allocations a ON a.id = s.primary_allocation_id
		WHERE s.id = $1
	`, serverID).Scan(
		&target.ServerID,
		&target.EggID,
		&target.Name,
		&target.NodeURL,
		&nodeID,
		&tokenID,
		&daemonToken,
		&daemonTokenEncrypted,
		&target.Image,
		&target.StartupCommand,
		&target.InstallScript,
		&target.InstallContainer,
		&target.InstallEntrypoint,
		&target.ConfigJSON,
		&target.FileDenylist,
		&target.MemoryMB,
		&target.SwapMB,
		&target.CPUShares,
		&target.CPULimit,
		&target.DiskMB,
		&target.IOWeight,
		&target.Threads,
		&target.OOMDisabled,
		&allocationIP,
		&allocationPort,
		&target.Suspended,
		&target.Installed,
		&target.Status,
	)
	if err != nil {
		return ServerProvisionTarget{}, err
	}
	token, err := s.decryptSecret(daemonTokenEncrypted, daemonToken, secretAAD("nodes", nodeID, "daemon_token"))
	if err != nil {
		return ServerProvisionTarget{}, err
	}
	if tokenID != "" && token != "" {
		target.NodeToken = tokenID + "." + token
	}
	if allocationIP.Valid {
		target.AllocationIP = allocationIP.String
	}
	if allocationPort.Valid {
		target.AllocationPort = int(allocationPort.Int64)
	}
	target.Environment = map[string]string{}
	variableRows, err := s.db.Query(ctx, `
		SELECT ev.env_variable, COALESCE(sv.variable_value, ev.default_value)
		FROM servers srv
		JOIN egg_variables ev ON ev.egg_id = srv.egg_id
		LEFT JOIN server_variables sv ON sv.server_id = srv.id AND sv.variable_id = ev.id
		WHERE srv.id = $1
		ORDER BY ev.env_variable
	`, serverID)
	if err != nil {
		return ServerProvisionTarget{}, err
	}
	for variableRows.Next() {
		var key, value string
		if err := variableRows.Scan(&key, &value); err != nil {
			variableRows.Close()
			return ServerProvisionTarget{}, err
		}
		target.Environment[key] = value
	}
	if err := variableRows.Err(); err != nil {
		variableRows.Close()
		return ServerProvisionTarget{}, err
	}
	variableRows.Close()
	target.StartupCommand = resolveStartupCommand(target.StartupCommand, target.Environment)
	target.Mounts, err = s.ServerMounts(ctx, serverID)
	if err != nil {
		return ServerProvisionTarget{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT a.id::text, host(a.ip), a.port, a.container_port, a.protocol
		FROM allocations a JOIN servers s ON s.id=a.server_id
		WHERE a.server_id=$1 AND a.node_id=s.node_id
		ORDER BY (a.id=s.primary_allocation_id) DESC, a.port`, serverID)
	if err != nil {
		return ServerProvisionTarget{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var allocation ServerRuntimeAllocation
		if err := rows.Scan(&allocation.ID, &allocation.IP, &allocation.Port, &allocation.ContainerPort, &allocation.Protocol); err != nil {
			return ServerProvisionTarget{}, err
		}
		target.Allocations = append(target.Allocations, allocation)
	}
	if err := rows.Err(); err != nil {
		return ServerProvisionTarget{}, err
	}
	return target, nil
}

// SetServerProvisioned records that Beacon accepted and created the workload.
// Installation remains a separate explicit lifecycle action.
func (s *Store) SetServerProvisioned(ctx context.Context, serverID string) error {
	commandTag, err := s.db.Exec(ctx, `UPDATE servers SET status = 'created', actual_state = 'stopped', installed = false, install_error = NULL, updated_at = now() WHERE id = $1`, serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	return s.AppendAudit(ctx, nil, "server workload created", "server", &serverID, `{"installed":false}`)
}

func (s *Store) SetServerInstallState(ctx context.Context, serverID, state, errorText string) error {
	var query string
	var args []any
	switch state {
	case "installing":
		query = `UPDATE servers SET status = 'installing', installed = false, install_started_at = now(), install_completed_at = NULL, install_failed_at = NULL, install_error = NULL WHERE id = $1`
		args = []any{serverID}
	case "installed":
		query = `UPDATE servers SET status = 'stopped', installed = true, install_completed_at = now(), installed_at = now(), install_failed_at = NULL, install_error = NULL WHERE id = $1`
		args = []any{serverID}
	case "failed":
		query = `UPDATE servers SET status = 'install_failed', installed = false, install_failed_at = now(), install_error = $2 WHERE id = $1`
		args = []any{serverID, errorText}
	default:
		return fmt.Errorf("invalid install state %q", state)
	}
	commandTag, err := s.db.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	return s.AppendAudit(ctx, nil, "server install "+state, "server", &serverID, fmt.Sprintf(`{"state":"%s"}`, state))
}

func (s *Store) MarkServerConfigSynced(ctx context.Context, serverID string) error {
	commandTag, err := s.db.Exec(ctx, `UPDATE servers SET last_config_sync_at = now(), config_sync_pending = false, config_sync_error = NULL WHERE id = $1`, serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	return nil
}

func (s *Store) MarkServerConfigSyncFailed(ctx context.Context, serverID, message string) error {
	commandTag, err := s.db.Exec(ctx, `UPDATE servers SET config_sync_pending = true, config_sync_error = $2 WHERE id = $1`, serverID, message)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	return nil
}

func (s *Store) SetServerStatus(ctx context.Context, serverID, status, action string) error {
	commandTag, err := s.db.Exec(ctx, `UPDATE servers SET status = $1 WHERE id = $2`, status, serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	return s.AppendAudit(ctx, nil, action, "server", &serverID, fmt.Sprintf(`{"status":"%s"}`, status))
}
