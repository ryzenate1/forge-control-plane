package store

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) ListMounts(ctx context.Context) ([]Mount, error) {
	rows, err := s.db.Query(ctx, `
		SELECT m.id::text, m.uuid::text, m.name, m.description, m.source, m.target, m.read_only, m.user_mountable,
		       COALESCE(array_remove(array_agg(DISTINCT mn.node_id::text), NULL), '{}'),
		       COALESCE(array_remove(array_agg(DISTINCT em.egg_id::text), NULL), '{}'),
		       COALESCE(array_remove(array_agg(DISTINCT ms.server_id::text), NULL), '{}')
		FROM mounts m
		LEFT JOIN mount_node mn ON mn.mount_id = m.id
		LEFT JOIN egg_mount em ON em.mount_id = m.id
		LEFT JOIN mount_server ms ON ms.mount_id = m.id
		GROUP BY m.id
		ORDER BY m.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	mounts := []Mount{}
	for rows.Next() {
		var mount Mount
		if err := rows.Scan(&mount.ID, &mount.UUID, &mount.Name, &mount.Description, &mount.Source, &mount.Target, &mount.ReadOnly, &mount.UserMountable, &mount.NodeIDs, &mount.TemplateIDs, &mount.ServerIDs); err != nil {
			return nil, err
		}
		mounts = append(mounts, mount)
	}
	return mounts, rows.Err()
}

func (s *Store) CreateMount(ctx context.Context, req CreateMountRequest, actorID *string) (Mount, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Source = strings.TrimSpace(req.Source)
	req.Target = strings.TrimSpace(req.Target)
	if req.Name == "" || req.Source == "" || req.Target == "" {
		return Mount{}, errors.New("name, source, and target are required")
	}
	if err := validateMountPaths(req.Source, req.Target); err != nil {
		return Mount{}, err
	}
	id := uuid.NewString()
	mountUUID := uuid.NewString()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Mount{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO mounts (id, uuid, name, description, source, target, read_only, user_mountable)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, id, mountUUID, req.Name, strings.TrimSpace(req.Description), req.Source, req.Target, req.ReadOnly, req.UserMountable); err != nil {
		return Mount{}, err
	}
	for _, nodeID := range req.NodeIDs {
		if strings.TrimSpace(nodeID) != "" {
			if _, err := tx.Exec(ctx, `INSERT INTO mount_node (mount_id, node_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, nodeID); err != nil {
				return Mount{}, err
			}
		}
	}
	for _, templateID := range req.TemplateIDs {
		if strings.TrimSpace(templateID) != "" {
			if _, err := tx.Exec(ctx, `INSERT INTO egg_mount (mount_id, egg_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, templateID); err != nil {
				return Mount{}, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Mount{}, err
	}
	_ = s.AppendAudit(ctx, actorID, "mount created", "mount", &id, fmt.Sprintf(`{"source":"%s","target":"%s"}`, req.Source, req.Target))
	return s.GetMount(ctx, id)
}

func (s *Store) GetMount(ctx context.Context, mountID string) (Mount, error) {
	rows, err := s.db.Query(ctx, `
		SELECT m.id::text, m.uuid::text, m.name, m.description, m.source, m.target, m.read_only, m.user_mountable,
		       COALESCE(array_remove(array_agg(DISTINCT mn.node_id::text), NULL), '{}'),
		       COALESCE(array_remove(array_agg(DISTINCT em.egg_id::text), NULL), '{}'),
		       COALESCE(array_remove(array_agg(DISTINCT ms.server_id::text), NULL), '{}')
		FROM mounts m
		LEFT JOIN mount_node mn ON mn.mount_id = m.id
		LEFT JOIN egg_mount em ON em.mount_id = m.id
		LEFT JOIN mount_server ms ON ms.mount_id = m.id
		WHERE m.id = $1
		GROUP BY m.id
	`, mountID)
	if err != nil {
		return Mount{}, err
	}
	defer rows.Close()
	if rows.Next() {
		var mount Mount
		if err := rows.Scan(&mount.ID, &mount.UUID, &mount.Name, &mount.Description, &mount.Source, &mount.Target, &mount.ReadOnly, &mount.UserMountable, &mount.NodeIDs, &mount.TemplateIDs, &mount.ServerIDs); err != nil {
			return Mount{}, err
		}
		return mount, nil
	}
	return Mount{}, errors.New("mount not found")
}

type UpdateMountRequest struct {
	Name          *string
	Description   *string
	Source        *string
	Target        *string
	ReadOnly      *bool
	UserMountable *bool
}

func (s *Store) UpdateMount(ctx context.Context, mountID string, req UpdateMountRequest) (Mount, error) {
	if req.Source != nil {
		source := strings.TrimSpace(*req.Source)
		if err := validateMountPath(source, "source"); err != nil {
			return Mount{}, err
		}
		req.Source = &source
	}
	if req.Target != nil {
		target := strings.TrimSpace(*req.Target)
		if err := validateMountPath(target, "target"); err != nil {
			return Mount{}, err
		}
		req.Target = &target
	}

	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.Source != nil {
		setClauses = append(setClauses, fmt.Sprintf("source = $%d", argIdx))
		args = append(args, *req.Source)
		argIdx++
	}
	if req.Target != nil {
		setClauses = append(setClauses, fmt.Sprintf("target = $%d", argIdx))
		args = append(args, *req.Target)
		argIdx++
	}
	if req.ReadOnly != nil {
		setClauses = append(setClauses, fmt.Sprintf("read_only = $%d", argIdx))
		args = append(args, *req.ReadOnly)
		argIdx++
	}
	if req.UserMountable != nil {
		setClauses = append(setClauses, fmt.Sprintf("user_mountable = $%d", argIdx))
		args = append(args, *req.UserMountable)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetMount(ctx, mountID)
	}

	args = append(args, mountID)
	query := fmt.Sprintf("UPDATE mounts SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx)
	if _, err := s.db.Exec(ctx, query, args...); err != nil {
		return Mount{}, err
	}
	return s.GetMount(ctx, mountID)
}

func (s *Store) AttachEggToMount(ctx context.Context, mountID, eggID string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO egg_mount (mount_id, egg_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, mountID, eggID)
	return err
}

func (s *Store) DetachEggFromMount(ctx context.Context, mountID, eggID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM egg_mount WHERE mount_id = $1 AND egg_id = $2`, mountID, eggID)
	return err
}

func (s *Store) AttachNodeToMount(ctx context.Context, mountID, nodeID string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO mount_node (mount_id, node_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, mountID, nodeID)
	return err
}

func (s *Store) DetachNodeFromMount(ctx context.Context, mountID, nodeID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM mount_node WHERE mount_id = $1 AND node_id = $2`, mountID, nodeID)
	return err
}

// AttachServerToMount records that a server may use a mount only when the
// mount is eligible for both the server's node and egg.
func (s *Store) AttachServerToMount(ctx context.Context, mountID, serverID string) error {
	return s.assignMountToServer(ctx, serverID, mountID, nil)
}

// DetachServerFromMount removes a server-mount link.
func (s *Store) DetachServerFromMount(ctx context.Context, mountID, serverID string) error {
	return s.removeMountFromServer(ctx, serverID, mountID, nil)
}

// ServerMountsForMount lists the servers directly attached to a mount.
func (s *Store) ServerMountsForMount(ctx context.Context, mountID string) ([]Server, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id::text, s.name, s.status, s.memory_mb, s.cpu_shares, s.disk_mb,
		       n.name, u.email, e.name
		FROM mount_server sm
		JOIN servers s ON s.id = sm.server_id
		JOIN nodes n ON n.id = s.node_id
		JOIN users u ON u.id = s.owner_id
		JOIN eggs e ON e.id = s.egg_id
		WHERE sm.mount_id = $1
		ORDER BY s.name
	`, mountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Server{}
	for rows.Next() {
		var server Server
		if err := rows.Scan(&server.ID, &server.Name, &server.Status, &server.MemoryMB, &server.CPUShares, &server.DiskMB, &server.Node, &server.Owner, &server.Template); err != nil {
			return nil, err
		}
		out = append(out, server)
	}
	return out, rows.Err()
}

func (s *Store) DeleteMount(ctx context.Context, mountID string, actorID *string) error {
	commandTag, err := s.db.Exec(ctx, `DELETE FROM mounts WHERE id = $1`, mountID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("mount not found")
	}
	return s.AppendAudit(ctx, actorID, "mount deleted", "mount", &mountID, `{"reason":"admin delete"}`)
}

func (s *Store) AssignMountToServer(ctx context.Context, serverID, mountID string, actorID *string) error {
	return s.assignMountToServer(ctx, serverID, mountID, actorID)
}

func (s *Store) assignMountToServer(ctx context.Context, serverID, mountID string, actorID *string) error {
	if err := s.ensureMountAvailableForServer(ctx, serverID, mountID); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO mount_server (server_id, mount_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, serverID, mountID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE servers SET config_sync_pending = true, config_sync_error = NULL, updated_at = now() WHERE id = $1`, serverID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return s.AppendAudit(ctx, actorID, "mount assigned to server", "server", &serverID, fmt.Sprintf(`{"mountId":"%s"}`, mountID))
}

func (s *Store) ensureMountAvailableForServer(ctx context.Context, serverID, mountID string) error {
	var allowed bool
	if err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM servers s
			JOIN mount_node mn ON mn.node_id = s.node_id AND mn.mount_id = $2
			JOIN egg_mount em ON em.egg_id = s.egg_id AND em.mount_id = $2
			WHERE s.id = $1
		)
	`, serverID, mountID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return errors.New("mount is not available for this server node and egg")
	}
	return nil
}

func validateMountPaths(source, target string) error {
	if err := validateMountPath(source, "source"); err != nil {
		return err
	}
	return validateMountPath(target, "target")
}

func validateMountPath(value, field string) error {
	if !path.IsAbs(value) || path.Clean(value) != value || strings.Contains(value, "\\") {
		return fmt.Errorf("mount %s must be an absolute, clean path", field)
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return fmt.Errorf("mount %s must not contain '..'", field)
		}
	}
	if field == "source" && (value == "/etc/forge" || value == "/var/lib/forge/volumes") {
		return errors.New("mount source or target is reserved")
	}
	if field == "target" && (value == "/" || value == "/home/container") {
		return errors.New("mount source or target is reserved")
	}
	return nil
}

func (s *Store) RemoveMountFromServer(ctx context.Context, serverID, mountID string, actorID *string) error {
	return s.removeMountFromServer(ctx, serverID, mountID, actorID)
}

func (s *Store) removeMountFromServer(ctx context.Context, serverID, mountID string, actorID *string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	commandTag, err := tx.Exec(ctx, `DELETE FROM mount_server WHERE server_id = $1 AND mount_id = $2`, serverID, mountID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("mount assignment not found")
	}
	if _, err := tx.Exec(ctx, `UPDATE servers SET config_sync_pending = true, config_sync_error = NULL, updated_at = now() WHERE id = $1`, serverID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return s.AppendAudit(ctx, actorID, "mount removed from server", "server", &serverID, fmt.Sprintf(`{"mountId":"%s"}`, mountID))
}

func (s *Store) AllowedMountSourcesForNode(ctx context.Context, nodeID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT m.source
		FROM mounts m
		JOIN mount_node mn ON mn.mount_id = m.id
		WHERE mn.node_id = $1
		ORDER BY m.source
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := []string{}
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

// ListEggsForMount returns eggs linked to a mount via the pivot table.
func (s *Store) ListEggsForMount(ctx context.Context, mountID string) ([]Egg, error) {
	rows, err := s.db.Query(ctx, `
		SELECT e.id::text, e.nest_id::text, n.name, e.name, COALESCE(e.description, ''),
		       e.docker_images, e.startup, e.config, e.default_memory_mb,
		       e.install_script, e.install_container, e.install_entrypoint, e.file_denylist,
		       e.config_from::text, e.copy_script_from::text,
		       COALESCE(e.update_url, ''), COALESCE(e.author, ''),
		       e.features, e.startup_commands, e.created_at
		FROM eggs e
		JOIN nests n ON n.id = e.nest_id
		JOIN egg_mount em ON em.egg_id = e.id
		WHERE em.mount_id = $1
		ORDER BY n.name, e.name
	`, mountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	eggs := []Egg{}
	for rows.Next() {
		var egg Egg
		if err := rows.Scan(&egg.ID, &egg.NestID, &egg.NestName, &egg.Name, &egg.Description,
			&egg.DockerImages, &egg.Startup, &egg.Config, &egg.DefaultMemoryMB, &egg.InstallScript,
			&egg.InstallContainer, &egg.InstallEntrypoint, &egg.FileDenylist,
			&egg.ConfigFrom, &egg.CopyScriptFrom,
			&egg.UpdateURL, &egg.Author,
			&egg.Features, &egg.StartupCommands, &egg.CreatedAt); err != nil {
			return nil, err
		}
		eggs = append(eggs, egg)
	}
	return eggs, rows.Err()
}

// ListNodesForMount returns nodes linked to a mount via the pivot table.
func (s *Store) ListNodesForMount(ctx context.Context, mountID string) ([]Node, error) {
	rows, err := s.db.Query(ctx, `
		SELECT n.id::text, COALESCE(n.uuid, n.id)::text, n.name, COALESCE(l.long, n.region), n.base_url,
		       COALESCE(n.fqdn, ''), COALESCE(n.scheme, 'http'), n.behind_proxy, n.status, n.maintenance_mode,
		       COALESCE(n.memory_mb, 0), COALESCE(n.disk_mb, 0), COALESCE(n.upload_size_mb, 100),
		       COALESCE(n.daemon_base, '/var/lib/forge/volumes'),
		       COALESCE(n.daemon_listen, 8080), COALESCE(n.daemon_sftp, 2022),
		       COALESCE(n.daemon_token_id, ''),
		       n.last_seen_at,
		       n.version, n.os, n.architecture, n.cpu_threads, n.docker_status, n.node_memory_mb, n.node_disk_mb, n.heartbeat_error,
		       COALESCE(n.display_name, ''), COALESCE(n.public_hostname, ''),
		       COALESCE(n.listen_port_min, 0), COALESCE(n.listen_port_max, 0),
		       COALESCE(n.allowed_ips, '{}'), COALESCE(n.network_interface, ''),
		       COALESCE(n.daemon_ssl_cert, ''), COALESCE(n.daemon_ssl_key, ''),
		       n.auto_connect, COALESCE(n.connection_retries, 3), COALESCE(n.heartbeat_interval, 15),
		       COALESCE(n.cpu_cores, 0), n.cpu_threads,
		       COALESCE(n.memory_overallocate, 0), COALESCE(n.disk_overallocate, 0),
		       COALESCE(n.reserved_memory_mb, 0), COALESCE(n.reserved_disk_mb, 0),
		       COALESCE(n.default_allocation_ip, '0.0.0.0'),
		       COALESCE(n.allocation_port_min, 25565), COALESCE(n.allocation_port_max, 26565),
		       n.auto_allocate,
		       COALESCE(n.backup_directory, ''), COALESCE(n.transfer_directory, ''),
		       COALESCE(n.mount_points, '[]'), COALESCE(n.token_rotation_policy, 'manual'),
		       COALESCE(n.firewall_rules, '[]'), COALESCE(n.tls_setting, 'auto'),
		       n.enable_health_checks, n.enable_metrics,
		       COALESCE(n.prometheus_endpoint, ''),
		       COALESCE(n.alert_threshold_cpu, 90), COALESCE(n.alert_threshold_memory, 90), COALESCE(n.alert_threshold_disk, 90),
		   COALESCE(n.maintenance_message, ''), n.drain_before_maintenance,
		   COALESCE(n.labels, '[]'), COALESCE(n.cluster_group_id, ''),
		   COALESCE(n.public, true),
		       COALESCE(n.description, ''), n.location_id::text, n.region_id::text,
		   COALESCE(n.draining, false), COALESCE(n.desired_state, 'active'), COALESCE(n.actual_state, 'offline'),
	       COALESCE(n.heartbeat_state::text, ''), COALESCE(n.heartbeat_recovery_count, 0),
	       COALESCE(n.daemon_sftp_alias, ''), COALESCE(n.daemon_connect, 8080), COALESCE(n.cpu_overallocate, 0),
	       COALESCE(n.tags, '[]')
		FROM nodes n
		LEFT JOIN locations l ON l.id = n.location_id
		JOIN mount_node mn ON mn.node_id = n.id
		WHERE mn.mount_id = $1
		ORDER BY n.name
	`, mountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := []Node{}
	for rows.Next() {
		var node Node
		if err := rows.Scan(
			&node.ID, &node.UUID, &node.Name, &node.Region, &node.BaseURL,
			&node.FQDN, &node.Scheme, &node.BehindProxy, &node.Status, &node.Maintenance,
			&node.MemoryMB, &node.DiskMB, &node.UploadSizeMB, &node.DaemonBase,
			&node.DaemonListen, &node.DaemonSFTP, &node.TokenID, &node.LastSeenAt,
			&node.Version, &node.OS, &node.Architecture, &node.CPUThreads, &node.DockerStatus,
			&node.NodeMemoryMB, &node.NodeDiskMB, &node.HeartbeatErr,
			&node.DisplayName, &node.PublicHostname,
			&node.ListenPortMin, &node.ListenPortMax,
			&node.AllowedIPs, &node.NetworkInterface,
			&node.DaemonSSLCert, &node.DaemonSSLKey,
			&node.AutoConnect, &node.ConnectionRetries, &node.HeartbeatInterval,
			&node.CPUCores, &node.CPUThreads,
			&node.MemoryOverallocate, &node.DiskOverallocate,
			&node.ReservedMemoryMB, &node.ReservedDiskMB,
			&node.DefaultAllocationIP,
			&node.AllocationPortMin, &node.AllocationPortMax,
			&node.AutoAllocate,
			&node.BackupDirectory, &node.TransferDirectory,
			&node.MountPoints, &node.TokenRotationPolicy,
			&node.FirewallRules, &node.TLSSetting,
			&node.EnableHealthChecks, &node.EnableMetrics,
			&node.PrometheusEndpoint,
			&node.AlertThresholdCPU, &node.AlertThresholdMemory, &node.AlertThresholdDisk,
			&node.MaintenanceMessage, &node.DrainBeforeMaintenance,
			&node.Labels, &node.ClusterGroupID,
			&node.Public,
			&node.Description, &node.LocationID, &node.RegionID,
			&node.Draining, &node.DesiredState, &node.ActualState,
			&node.HeartbeatState, &node.HeartbeatRecoveryCount,
			&node.DaemonSFTPAlias, &node.DaemonConnect, &node.CPUOverallocate,
			&node.Tags); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *Store) ServerMounts(ctx context.Context, serverID string) ([]ServerMount, error) {
	rows, err := s.db.Query(ctx, `
		SELECT m.id::text, m.name, m.source, m.target, m.read_only
		FROM mounts m
		JOIN mount_server ms ON ms.mount_id = m.id
		WHERE ms.server_id = $1
		ORDER BY m.name
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	mounts := []ServerMount{}
	for rows.Next() {
		var mount ServerMount
		if err := rows.Scan(&mount.ID, &mount.Name, &mount.Source, &mount.Target, &mount.ReadOnly); err != nil {
			return nil, err
		}
		mounts = append(mounts, mount)
	}
	return mounts, rows.Err()
}
